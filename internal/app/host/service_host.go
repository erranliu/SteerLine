package host

import (
	"errors"
	"io"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/hostport"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

// ServiceHost owns file transport and console publication. It holds no
// domain writer, task result capability or mutable execution records.
type ServiceHost struct {
	Config          Config
	Console         string
	consoleOut      io.Writer
	formatter       ActivityFormatter
	consoleSequence uint64
}

func NewServiceHost(c Config, console string, output io.Writer, formatter ActivityFormatter) *ServiceHost {
	return &ServiceHost{Config: c, Console: console, consoleOut: output, formatter: formatter}
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

func (s *ServiceHost) CurrentProcessIdentity() (*executionbackend.ProcessIdentity, error) {
	if s.Config.ProcessFacts == nil {
		return nil, errors.New("application process-facts port is required")
	}
	return s.Config.ProcessFacts.CurrentIdentity()
}

var _ hostport.HostPort = (*ServiceHost)(nil)
