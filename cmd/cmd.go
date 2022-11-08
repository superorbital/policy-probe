package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superorbital/kubectl-probe/pkg/api"
	"github.com/superorbital/kubectl-probe/pkg/probe"
	"github.com/superorbital/kubectl-probe/pkg/test"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func New() *cobra.Command {
	var cfgFile string
	var debug bool
	var config api.TestSuite
	cmd := &cobra.Command{
		Use:   "kubectl-probe",
		Short: "A brief description of your application",
		Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		SilenceUsage: true, // Don't show usage on errors
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			loggerConfig := zap.Config{
				Level:            zap.NewAtomicLevelAt(zap.InfoLevel),
				Encoding:         "json",
				EncoderConfig:    zap.NewProductionEncoderConfig(),
				OutputPaths:      []string{"stderr"},
				ErrorOutputPaths: []string{"stderr"},
			}
			if debug {
				loggerConfig.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
			}

			logger := zap.Must(loggerConfig.Build())
			zap.ReplaceGlobals(logger)

			ctx, cancel := context.WithCancel(cmd.Context())
			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt)
			go func() {
				select {
				case <-c:
					cancel()
				case <-ctx.Done():
				}
			}()

			cmd.SetContext(ctx)
		},
		Run: func(cmd *cobra.Command, args []string) {
			viper.SetConfigFile(cfgFile)
			if err := viper.ReadInConfig(); err != nil {
				log.Fatalf("reading config: %v", err)
			}
			if err := viper.Unmarshal(&config); err != nil {
				log.Fatalf("failed to parse config: %v", err)
				return
			}
			clientset, err := newClientset()
			if err != nil {
				log.Fatalf("failed to create client: %v", err)
				return
			}
			if err := test.Run(cmd.Context(), config, test.NewFactory(probe.NewFactory(clientset, config.Spec.ProbeImage))); err != nil {
				zap.L().Fatal("failed", zap.Error(err))
			}
			zap.L().Info("passed")
		},
	}
	viper.AutomaticEnv()
	cmd.Flags().StringVar(&cfgFile, "config", "", "config file")
	cmd.Flags().StringVar(&config.Spec.ProbeImage, "image", "ghcr.io/superorbital/kubectl-probe:latest", "image for the probe container")
	cmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
	cmd.AddCommand(Probe())
	return cmd
}

func newClientset() (*kubernetes.Clientset, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, nil)

	clientConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create client config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}
	return clientset, nil
}
