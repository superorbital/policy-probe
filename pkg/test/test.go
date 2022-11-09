package test

import (
	"context"

	"github.com/rotisserie/eris"
	"github.com/superorbital/kubectl-probe/pkg/api"
	"github.com/superorbital/kubectl-probe/pkg/probe"
	"go.uber.org/zap"
)

func Run(ctx context.Context, cfg api.TestSuite, factory *Factory) (bool, error) {
	passed := true
	for _, tc := range cfg.Spec.TestCases {
		zap.L().Info("running test", zap.String("description", tc.Description))
		source, err := factory.BuildSource(ctx, &tc)
		if err != nil {
			return false, eris.Wrap(err, "failed to create source")
		}
		if tc.Expect == api.PassExpectType {
			err = source.AssertReachable(ctx)
		} else {
			err = source.AssertUnreachable(ctx)
		}
		if err != nil {
			zap.L().Warn("failed", zap.String("description", tc.Description), zap.Error(err))
			passed = false
		} else {
			zap.L().Info("passed", zap.String("description", tc.Description))
		}
	}
	return passed, nil
}

type Factory struct {
	probes *probe.Factory
}

func NewFactory(probes *probe.Factory) *Factory {
	return &Factory{
		probes: probes,
	}
}

func (f *Factory) BuildSource(ctx context.Context, cfg *api.TestCase) (api.Source, error) {
	switch {
	case cfg.From.Deployment != nil:
		return f.probes.Create(ctx, cfg)
	default:
		panic("not implemented")
	}
}
