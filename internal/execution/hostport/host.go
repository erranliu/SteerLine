package hostport

import (
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

// HostPort carries control transport and process facts only. Qualification,
// intent, result binding, acknowledgment and service state remain in execution.
type HostPort interface {
	ControlRequestIDs() ([]string, error)
	ReadControlRequest(string) (productapi.ControlRequest, error)
	ReadControlReceipt(string) (*productapi.ControlReceipt, error)
	ControlRequestDigest(string) string
	WriteControlReceipt(productapi.ControlReceipt) error
	Emit(executionvalues.ActivityEvent)
	CurrentProcessIdentity() (*executionbackend.ProcessIdentity, error)
}
