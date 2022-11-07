package test

import (
	"context"
	"fmt"

	"github.com/superorbital/kubectl-probe/pkg/api"
	"github.com/superorbital/kubectl-probe/pkg/probe"
)

func Run(ctx context.Context, cfg api.TestSuite, factory *Factory) error {
	for _, tc := range cfg.Spec.TestCases {
		source, err := factory.BuildSource(ctx, &tc)
		if err != nil {
			return fmt.Errorf("failed to create from: %w", err)
		}
		if tc.Expect == api.PassExpectType {
			source.AssertReachable()
		} else {
			source.AssertUnreachable()
		}
	}
	return nil
}

type Factory struct {
	probes *probe.Factory
}

func (f *Factory) BuildSource(ctx context.Context, cfg *api.TestCase) (api.Source, error) {
	switch {
	case cfg.From.Probe != nil:
		return f.probes.Create(ctx, cfg)
	default:
		panic("not implemented")
	}
}
