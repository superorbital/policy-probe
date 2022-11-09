package test

import (
	"context"
	"fmt"

	"github.com/superorbital/kubectl-probe/pkg/api"
	"github.com/superorbital/kubectl-probe/pkg/probe"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/errors"
)

func Run(ctx context.Context, cfg api.TestSuite, factory *Factory) error {
	var errs []error
	for _, tc := range cfg.Spec.TestCases {
		zap.L().Info("running test", zap.String("description", tc.Description))
		source, err := factory.BuildSource(ctx, &tc)
		if err != nil {
			return fmt.Errorf("failed to create from: %w", err)
		}
		if tc.Expect == api.PassExpectType {
			err = source.AssertReachable(ctx)
		} else {
			err = source.AssertUnreachable(ctx)
		}
		if err != nil {
			errs = append(errs, err)
			zap.L().Info("failed", zap.String("description", tc.Description))
		} else {
			zap.L().Info("passed", zap.String("description", tc.Description))
		}
	}
	return errors.NewAggregate(errs)
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
