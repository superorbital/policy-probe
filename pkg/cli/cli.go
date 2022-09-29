package cli

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superorbital/kubectl-probe/pkg/config"
	"github.com/superorbital/kubectl-probe/pkg/probe"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	watchtools "k8s.io/client-go/tools/watch"
)

var (
	configPath string
)

func init() {
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

func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:              "",
		Short:            "",
		Long:             "",
		PersistentPreRun: initConfig,
		RunE: func(cmd *cobra.Command, args []string) error {
			return RootCmd(cmd.Context())
		},
	}
	cmd.PersistentFlags().StringVar(&configPath, "config", "", "path to the probe config file")
	cmd.MarkPersistentFlagRequired("config")

	return cmd
}

func RootCmd(ctx context.Context) error {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, nil)

	clientConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to create client config: %w", err)
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return fmt.Errorf("failed to create clienset: %w", err)
	}

	var config config.Probe
	if err := viper.UnmarshalKey("spec", &config, func(c *mapstructure.DecoderConfig) {
		c.ErrorUnused = true
	}); err != nil {
		return fmt.Errorf("failed to unmarshal probe: %v", err)
	}

	if err := config.Validate(); err != nil {
		return err
	}

	p := probe.New(config, clientset)
	if err := p.Install(ctx); err != nil {
		return err
	}

	ctx, cancel := watchtools.ContextWithOptionalTimeout(ctx, time.Minute)
	defer cancel()
	if err := p.Wait(ctx); err != nil {
		return err
	}

	log.Println("getting logs")
	logs, err := p.Logs(ctx)
	if err != nil {
		return err
	}
	defer logs.Close()

	if _, err := io.Copy(os.Stderr, logs); err != nil {
		return fmt.Errorf("failed to copy logs to stderr: %v", err)
	}

	return nil
}
