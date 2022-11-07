package cmd

import (
	"log"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superorbital/kubectl-probe/pkg/api"
	"github.com/superorbital/kubectl-probe/pkg/test"
)

func New() *cobra.Command {
	var cfgFile string
	cmd := &cobra.Command{
		Use:   "kubectl-probe",
		Short: "A brief description of your application",
		Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		SilenceUsage: true, // Don't show usage on errors
		Run: func(cmd *cobra.Command, args []string) {
			viper.SetConfigFile(cfgFile)
			if err := viper.ReadInConfig(); err != nil {
				log.Fatalf("reading config: %v", err)
			}
			var config api.TestSuite
			if err := viper.Unmarshal(&config); err != nil {
				log.Fatalf("failed to parse config: %v", err)
				return
			}
			if err := test.Run(cmd.Context(), config, &test.Factory{}); err != nil {
				log.Println(err)
			}
		},
	}
	viper.AutomaticEnv()
	cmd.Flags().StringVar(&cfgFile, "config", "", "config file")
	cmd.AddCommand(Probe())
	return cmd
}
