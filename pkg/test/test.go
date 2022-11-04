package test

import (
	"context"
	"fmt"

	"github.com/superorbital/kubectl-probe/pkg/api"
	"github.com/superorbital/kubectl-probe/pkg/probe"
)

func Run(ctx context.Context, cfg api.TestSuite, factory *Factory) error {
	for _, tc := range cfg.Spec.TestCases {
		from, err := factory.BuildSource(ctx, &tc.From)
		if err != nil {
			return fmt.Errorf("failed to create from: %w", err)
		}
		sink, err := factory.BuildSink(ctx, from, &tc.To)
		if err != nil {
			return fmt.Errorf("failed to create to: %w", err)
		}
		if tc.Expect == api.PassExpectType {
			from.AssertReachable(sink)
		} else {
			from.AssertUnreachable(sink)
		}
	}
	return nil
}

type Factory struct {
	probes *probe.Factory
}

func (f *Factory) BuildSource(ctx context.Context, cfg *api.FromKinds) (api.Source, error) {
	switch {
	case cfg.Probe != nil:
		return f.probes.Create(ctx, cfg.Probe)
	default:
		panic("not implemented")
	}
}

func (f *Factory) BuildSink(ctx context.Context, source api.Source, cfg *api.ToKinds) (api.Sink, error) {
	switch {
	case cfg.Probe != nil:
		cfg.Probe.Sink = source.Config()
		return f.probes.Create(ctx, cfg.Probe)
	default:
		panic("not implemented")
	}
}
