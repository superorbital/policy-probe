package test

import (
	"context"
	"fmt"

	"github.com/superorbital/kubectl-probe/pkg/api"
	"github.com/superorbital/kubectl-probe/pkg/probe"
)

func Run(ctx context.Context, cfg api.TestSuite, factory *Factory) error {
	for _, tc := range cfg.Spec.TestCases {
		from, err := factory.BuildFrom(&tc.From)
		if err != nil {
			return fmt.Errorf("failed to create from: %w", err)
		}
		to, err := factory.BuildTo(from, &tc.To)
		if err != nil {
			return fmt.Errorf("failed to create to: %w", err)
		}
		if tc.Expect == api.PassExpectType {
			from.AssertReachable(to)
		} else {
			from.AssertUnreachable(to)
		}
	}
	return nil
}

type Factory struct {
	probes *probe.Factory
}

func (f *Factory) BuildFrom(cfg *api.FromKinds) (api.Source, error) {
	switch {
	case cfg.Probe != nil:
		return f.probes.CreateSource(probe.Source(cfg))
	default:
		panic("not implemented")
	}
}

func (f *Factory) BuildTo(source api.Source, cfg *api.ToKinds) (api.Destination, error) {
	switch {
	case cfg.Probe != nil:
		return f.probes.CreateDestination(probe.Destination(source.Config(), cfg))
	default:
		panic("not implemented")
	}
}
