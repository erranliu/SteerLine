package execution

import (
	contracts "github.com/erranliu/SteerLine/internal/steerline/contracts"
)

import (
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"time"
)

// HostPort carries transport and diagnostics only. All qualification, intent,
// result binding and acknowledgement decisions stay with the execution actor.
type HostPort interface {
	ValidateStateLocation(Config) error
	ControlRequestIDs() ([]string, error)
	ReadControlRequest(string) (productapi.ControlRequest, error)
	ReadControlReceipt(string) (*productapi.ControlReceipt, error)
	ControlRequestDigest(string) string
	WriteControlReceipt(productapi.ControlReceipt) error
	FindStopRequest(string, map[string]bool) (*productapi.ControlRequest, string, time.Time)
	PublishReady(string, *executionbackend.ProcessIdentity, string) error
	Emit(contracts.ActivityEvent)
	CurrentProcessIdentity() (*executionbackend.ProcessIdentity, error)
}

// ResourcePort is an injected OS/file fact capability; it has no business writes.
type ResourcePort interface {
	WorkerTreeAbsent(executionbackend.ProcessIdentity) bool
	AtomicWrite(string, []byte) error
}
