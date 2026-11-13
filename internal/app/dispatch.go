package app

import (
	"context"
	"errors"

	"github.com/erranliu/SteerLine/internal/product/api"
)

func (p *runtimeProjectPort) readDispatch(ctx context.Context) (api.DispatchObservation, error) {
	view, err := p.actor.ObserveDispatch(materialReads(ctx), func(ctx context.Context) error {
		if p.readiness == nil {
			return errors.New("项目就绪状态待核实")
		}
		readiness, err := p.readiness.projectReadiness(ctx)
		if err != nil {
			return err
		}
		return requireProjectReadiness(readiness)
	})
	view.EnvironmentReadinessRequired = p.config.environmentPolicy.RequireReadiness()
	return view, err
}
