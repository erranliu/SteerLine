package hostport

import (
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/hostcontract"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

// HostPort carries control transport and process facts only. Application
// admission and result binding stay with the caller; hostcontract owns state.
type HostPort interface {
	ControlRequestIDs() ([]string, error)
	ReadControlRequest(string) (productapi.ControlRequest, error)
	ReadControlReceipt(string) (*productapi.ControlReceipt, error)
	ControlRequestDigest(string) string
	WriteControlReceipt(productapi.ControlReceipt) error
	Emit(executionvalues.ActivityEvent)
	CurrentProcessIdentity() (*hostcontract.ProcessIdentity, error)
}
