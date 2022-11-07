/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"time"

	"github.com/rcrowley/go-metrics"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superorbital/kubectl-probe/pkg/api"
	"github.com/superorbital/kubectl-probe/pkg/probe"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func Probe() *cobra.Command {
	var cfg api.ProbeConfig
	cmd := &cobra.Command{
		Use:   "probe",
		Short: "A brief description of your command",
		Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			logger := zap.Must(zap.NewProduction())
			zap.ReplaceGlobals(logger)

			go logMetrics()

			if err := viper.Unmarshal(&cfg); err != nil {
				zap.L().Fatal("failed to parse config", zap.Error(err))
			}
			probe.Run(ctx, cfg)
		},
	}
	cmd.Flags().StringVar((*string)(&cfg.Protocol), "protocol", "tcp", "tcp|udp")
	cmd.Flags().IntVar(&cfg.Port, "port", 0, "")
	cmd.Flags().StringVar(&cfg.Address, "address", "0.0.0.0", "")
	cmd.Flags().StringVar(&cfg.Message, "message", "hello world", "")
	cmd.Flags().DurationVar(&cfg.Interval, "interval", time.Minute, "interval between pings")
	viper.BindPFlags(cmd.Flags())
	return cmd
}
func logMetrics() {
	for range time.Tick(5 * time.Second) {
		var fields []zapcore.Field
		metrics.DefaultRegistry.Each(func(s string, i interface{}) {
			switch metric := i.(type) {
			case metrics.Counter:
				fields = append(fields, zap.Int64(s, metric.Count()))
			default:
				zap.L().Warn("unhandled metric kind", zap.String("kind", fmt.Sprintf("%T", metric)))
			}
		})
		zap.L().Info("metrics", fields...)
	}
}
