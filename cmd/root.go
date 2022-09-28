package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superorbital/kubectl-probe/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	watchtools "k8s.io/client-go/tools/watch"
)

var rootCmd = &cobra.Command{
	Use:              "",
	Short:            "",
	Long:             "",
	PersistentPreRun: initConfig,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()
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

		var ns string
		switch {
		case probe.From.Probe != nil:
			ns = probe.From.Probe.Namespace
		case probe.From.Deployment != nil:
			ns = probe.From.Deployment.Namespace
		default:
			log.Fatal("must have a from")
		}
		deployment, err := clientset.AppsV1().Deployments(ns).Get(ctx, probe.From.Deployment.Name, metav1.GetOptions{})
		if err != nil {
			log.Fatal(err.Error())
		}
		pods, err := clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
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

		containerName := fmt.Sprintf("probe-%s", rand.String(5))
		ec := &corev1.EphemeralContainer{
			EphemeralContainerCommon: corev1.EphemeralContainerCommon{
				Name:    containerName,
				Image:   "alpine:latest",
				Command: []string{"nc"},
				Args:    []string{"-vz", probe.To.Server.Host, strconv.Itoa(probe.To.Server.Port)},
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

		result, err := clientset.CoreV1().Pods(ns).Patch(ctx, pod.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{}, "ephemeralcontainers")
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
		ctx, cancel := watchtools.ContextWithOptionalTimeout(ctx, time.Minute)
		defer cancel()

		fieldSelector := fields.OneTermEqualSelector("metadata.name", pod.Name).String()
		lw := &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				options.FieldSelector = fieldSelector
				return clientset.CoreV1().Pods(ns).List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				options.FieldSelector = fieldSelector
				return clientset.CoreV1().Pods(ns).Watch(ctx, options)
			},
		}
		ev, err := watchtools.UntilWithSync(ctx, lw, &corev1.Pod{}, nil, func(ev watch.Event) (bool, error) {
			log.Printf("watch received event %q with object %T", ev.Type, ev.Object)
			switch ev.Type {
			case watch.Deleted:
				return false, errors.NewNotFound(schema.GroupResource{Resource: "pods"}, "")
			}

			p, ok := ev.Object.(*corev1.Pod)
			if !ok {
				return false, fmt.Errorf("watch did not return a pod: %v", ev.Object)
			}

			s := getContainerStatusByName(p, containerName)
			if s == nil {
				return false, nil
			}
			log.Printf("debug container status is %v", s)
			if s.State.Running != nil || s.State.Terminated != nil {
				return true, nil
			}
			log.Printf("container %s: %s", containerName, s.State.Waiting.Message)
			return false, nil
		})
		if err != nil {
			log.Fatalf("failed to watch pod: %v", err)
		}
		result = ev.Object.(*corev1.Pod)
		_ = result
		log.Println("getting logs")
		logs, err := clientset.CoreV1().Pods(ns).GetLogs(pod.Name, &corev1.PodLogOptions{
			SinceSeconds: iptr(60),
			Follow:       true,
			Container:    containerName,
		}).Stream(ctx)
		if err != nil {
			log.Fatalf("failed to get logs: %v", err)
		}
		if _, err := io.Copy(os.Stderr, logs); err != nil {
			log.Fatalf("failed to copy logs to stderr: %v", err)
		}
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

func getContainerStatusByName(pod *corev1.Pod, containerName string) *corev1.ContainerStatus {
	allContainerStatus := [][]corev1.ContainerStatus{pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses, pod.Status.EphemeralContainerStatuses}
	for _, statusSlice := range allContainerStatus {
		for i := range statusSlice {
			if statusSlice[i].Name == containerName {
				return &statusSlice[i]
			}
		}
	}
	return nil
}

func iptr(x int64) *int64 {
	return &x
}
