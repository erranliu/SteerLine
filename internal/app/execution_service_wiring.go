package app

import (
	"context"

	"github.com/erranliu/SteerLine/internal/hostcontract"
	"github.com/erranliu/SteerLine/internal/product/httpapi"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

type serviceReadyHook struct {
	*taskrunner.Actor
	ready func() error
}

func (s serviceReadyHook) Initialize(ctx context.Context, cancel context.CancelFunc) error {
	if err := s.Actor.Initialize(ctx, cancel); err != nil {
		return err
	}
	if s.ready != nil {
		return s.ready()
	}
	return nil
}

func localAPIProcessIdentity(value *hostcontract.ProcessIdentity) httpapi.ProcessIdentity {
	if value == nil {
		return httpapi.ProcessIdentity{}
	}
	return httpapi.ProcessIdentity{Kind: value.Kind, PID: value.PID, Created: value.Created, Executable: value.Executable}
}

func serviceReadyDiagnostic(service *taskrunner.Actor) string {
	if service == nil {
		return "service instance is unavailable"
	}
	if service.State.Service.Error != nil {
		return *service.State.Service.Error
	}
	if !service.State.Service.Ready {
		return "service initialization completed without publishing ready=true"
	}
	return ""
}
