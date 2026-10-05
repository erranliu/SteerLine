package host

import (
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

// ServiceConnection is host transport state used by the CLI/app to supervise
// control files. The client boundary projects it to scalar product fields.
type ServiceConnection struct {
	Snapshot                      ObservationSnapshot               `json:"snapshot"`
	Process                       productapi.ProcessProbe           `json:"process"`
	State                         productapi.ServiceConnectionState `json:"state"`
	IdentityMatched               bool                              `json:"identity_matched"`
	ConfigMatched                 bool                              `json:"config_matched"`
	Protocol                      int                               `json:"control_protocol"`
	Capabilities                  []string                          `json:"capabilities"`
	CanControl, CanStop, CanStart bool
	StartupID                     string `json:"startup_id,omitempty"`
	Diagnostic                    string `json:"diagnostic,omitempty"`
}
