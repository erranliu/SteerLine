package execution

import (
	contracts "github.com/erranliu/SteerLine/internal/steerline/contracts"
)

import (
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"regexp"
)

const ControlProtocolVersion = 1

var controlIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func (s *Service) writeReceipt(receipt productapi.ControlReceipt) error {
	return s.host.WriteControlReceipt(receipt)
}
func (s *Service) emitConsole(event contracts.ActivityEvent) { s.host.Emit(event) }
func (s *Service) publishStartupReady() error {
	if !s.State.Service.Ready {
		return nil
	}
	return s.host.PublishReady(s.State.Service.InstanceID, s.State.Service.ProcessIdentity, s.State.Control.DispatchMode)
}
