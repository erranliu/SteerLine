package host

import (
	"context"

	"github.com/erranliu/SteerLine/internal/execution"
)

// ApplicationAdmission injects configuration and persisted-state checks into
// the service host without giving the host a storage or backend implementation.
type ApplicationAdmission interface {
	ValidateStateLocation(execution.Config) error
	Preflight(context.Context, *execution.Config, bool) error
	InitializeNativeProject(context.Context, execution.Config) error
}
