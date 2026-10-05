package hostport

import (
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

// HostPort carries control transport and process facts only. Qualification,
// intent, result binding, acknowledgment and service state remain in execution.
type HostPort interface {
	ValidateStateLocation() error
	ControlRequestIDs() ([]string, error)
	ReadControlRequest(string) (productapi.ControlRequest, error)
	ReadControlReceipt(string) (*productapi.ControlReceipt, error)
	ControlRequestDigest(string) string
	WriteControlReceipt(productapi.ControlReceipt) error
	FindStopRequest(string, map[string]bool) (*productapi.ControlRequest, string, time.Time)
	PublishReady(string, *executionbackend.ProcessIdentity, string) error
	Emit(executionvalues.ActivityEvent)
	CurrentProcessIdentity() (*executionbackend.ProcessIdentity, error)
}
