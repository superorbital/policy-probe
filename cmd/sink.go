package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superorbital/kubectl-probe/pkg/api"
	"github.com/superorbital/kubectl-probe/pkg/probe"
)

func Sink() *cobra.Command {
	var cfg api.ProbeConfig
	cmd := &cobra.Command{
		Use:   "sink",
		Short: "A brief description of your command",
		Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := viper.Unmarshal(&cfg); err != nil {
				return fmt.Errorf("failed to parse config: %w", err)
			}
			return probe.SinkCmd(cmd.Context(), cfg)
		},
	}
	cmd.Flags().StringVar((*string)(&cfg.Protocol), "protocol", "tcp", "tcp|udp")
	cmd.Flags().IntVar(&cfg.Port, "port", 0, "")
	cmd.Flags().StringVar(&cfg.Address, "address", "0.0.0.0", "")
	cmd.Flags().StringVar(&cfg.Message, "message", "hello world", "")
	viper.BindPFlags(cmd.Flags())
	return cmd
}
