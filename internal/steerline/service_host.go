package steerline

import (
	"io"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

// ServiceHost owns file transport and console/startup publication. It holds no
// domain writer, task result capability or mutable execution records.
type ServiceHost struct {
	Config          execution.Config
	Console         string
	consoleOut      io.Writer
	consoleSequence uint64
	startupID       string
}

func NewServiceHost(c execution.Config, startupID, console string, output io.Writer) *ServiceHost {
	return &ServiceHost{Config: c, startupID: startupID, Console: console, consoleOut: output}
}
func (*ServiceHost) ValidateStateLocation(c execution.Config) error {
	return validateNativeStateLocation(c)
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
func (*ServiceHost) CurrentProcessIdentity() (*executionbackend.ProcessIdentity, error) {
	return resourceprocess.CurrentIdentity()
}

type ExecutionResources struct{}

func (ExecutionResources) WorkerTreeAbsent(id executionbackend.ProcessIdentity) bool {
	return resourceprocess.WorkerTreeAbsent(id)
}
func (ExecutionResources) AtomicWrite(path string, data []byte) error { return AtomicWrite(path, data) }

var _ execution.HostPort = (*ServiceHost)(nil)
var _ execution.ResourcePort = ExecutionResources{}
