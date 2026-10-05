package host

import (
	"errors"
	"io"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

// ServiceHost owns file transport and console/startup publication. It holds no
// domain writer, task result capability or mutable execution records.
type ServiceHost struct {
	Config          execution.Config
	Console         string
	consoleOut      io.Writer
	formatter       ActivityFormatter
	admission       ApplicationAdmission
	consoleSequence uint64
	startupID       string
}

func NewServiceHost(c execution.Config, startupID, console string, output io.Writer, admission ApplicationAdmission, formatter ActivityFormatter) *ServiceHost {
	return &ServiceHost{Config: c, startupID: startupID, Console: console, consoleOut: output, admission: admission, formatter: formatter}
}
func (s *ServiceHost) ValidateStateLocation(c execution.Config) error {
	if s.admission == nil {
		return errors.New("application state-location validator is required")
	}
	return s.admission.ValidateStateLocation(c)
}
func (s *ServiceHost) ReadControlRequest(id string) (productapi.ControlRequest, error) {
	return readControlRequest(controlRequestPath(s.Config.RunDir, id))
}
func (s *ServiceHost) ReadControlReceipt(id string) (*productapi.ControlReceipt, error) {
	return ReadControlReceipt(s.Config, id)
}
func (s *ServiceHost) ControlRequestDigest(id string) string {
	return rawRequestDigest(controlRequestPath(s.Config.RunDir, id))
}
func (s *ServiceHost) FindStopRequest(instance string, seen map[string]bool) (*productapi.ControlRequest, string, time.Time) {
	return findStopRequestCached(s.Config, instance, seen)
}
func (s *ServiceHost) CurrentProcessIdentity() (*executionbackend.ProcessIdentity, error) {
	if s.Config.ProcessFacts == nil {
		return nil, errors.New("application process-facts port is required")
	}
	return s.Config.ProcessFacts.CurrentIdentity()
}

var _ execution.HostPort = (*ServiceHost)(nil)
