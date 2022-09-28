package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superorbital/kubectl-probe/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var rootCmd = &cobra.Command{
	Use:              "",
	Short:            "",
	Long:             "",
	PersistentPreRun: initConfig,
	Run: func(cmd *cobra.Command, args []string) {
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, nil)

		clientConfig, err := kubeConfig.ClientConfig()
		if err != nil {
			log.Fatal(err.Error())
		}

		// create the clientset
		clientset, err := kubernetes.NewForConfig(clientConfig)
		if err != nil {
			log.Fatal(err.Error())
		}
		var probe config.Probe
		if err := viper.UnmarshalKey("spec", &probe, func(c *mapstructure.DecoderConfig) {
			c.ErrorUnused = true
		}); err != nil {
			log.Fatalf("failed to unmarshal probe: %v", err)
		}

		var namespace string
		switch {
		case probe.From.Probe != nil:
			namespace = probe.From.Probe.Namespace
		case probe.From.Deployment != nil:
			namespace = probe.From.Deployment.Namespace
		default:
			log.Fatal("must have a from")
		}
		deployment, err := clientset.AppsV1().Deployments(namespace).Get(cmd.Context(), probe.From.Deployment.Name, metav1.GetOptions{})
		if err != nil {
			log.Fatal(err.Error())
		}
		pods, err := clientset.CoreV1().Pods(namespace).List(cmd.Context(), metav1.ListOptions{
			LabelSelector: metav1.FormatLabelSelector(deployment.Spec.Selector),
		})
		if err != nil {
			log.Fatal(err.Error())
		}
		if len(pods.Items) < 1 {
			log.Fatal("could not find a matching pod")
		}

		pod := pods.Items[0]
		podJS, err := json.Marshal(pod)
		if err != nil {
			log.Fatalf("error creating JSON for pod: %v", err)
		}

		copied := pod.DeepCopy()

		ec := &corev1.EphemeralContainer{
			EphemeralContainerCommon: corev1.EphemeralContainerCommon{
				Name:  "foo",
				Image: "alpine:latest",
			},
		}
		copied.Spec.EphemeralContainers = append(copied.Spec.EphemeralContainers, *ec)
		debugJS, err := json.Marshal(copied)
		if err != nil {
			log.Fatalf("error creating JSON for debug container: %v", err)
		}

		patch, err := strategicpatch.CreateTwoWayMergePatch(podJS, debugJS, pod)
		if err != nil {
			log.Fatalf("error creating patch to add debug container: %v", err)
		}
		log.Printf("generated strategic merge patch for debug container: %s", patch)

		result, err := clientset.CoreV1().Pods(namespace).Patch(cmd.Context(), pod.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{}, "ephemeralcontainers")
		if err != nil {
			// The apiserver will return a 404 when the EphemeralContainers feature is disabled because the `/ephemeralcontainers` subresource
			// is missing. Unlike the 404 returned by a missing pod, the status details will be empty.
			if serr, ok := err.(*errors.StatusError); ok && serr.Status().Reason == metav1.StatusReasonNotFound && serr.ErrStatus.Details.Name == "" {
				log.Fatalf("ephemeral containers are disabled for this cluster (error from server: %q).", err)
			}

			// The Kind used for the /ephemeralcontainers subresource changed in 1.22. When presented with an unexpected
			// Kind the api server will respond with a not-registered error.
			if runtime.IsNotRegisteredError(err) {
				log.Fatalf("ephemeral containers are disabled for this cluster (error from server: %q).", err)
			}
		}
		_ = result
		log.Printf("successfully started ephemeral container")
	},
}

var (
	configPath string
)

func init() {
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "path to the probe config file")
	rootCmd.MarkPersistentFlagRequired("config")
}

func initConfig(*cobra.Command, []string) {
	viper.SetConfigFile(configPath)

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// handled by required flag?
		} else {
			log.Fatal(err.Error())
		}
	}
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
