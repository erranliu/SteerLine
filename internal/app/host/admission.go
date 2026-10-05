package host

import (
	"context"
)

// ApplicationAdmission injects configuration and persisted-state checks into
// the service host without giving the host a storage or backend implementation.
type ApplicationAdmission interface {
	ValidateStateLocation(Config) error
	Preflight(context.Context, Config, bool) (Config, error)
	InitializeNativeProject(context.Context, Config) error
}
