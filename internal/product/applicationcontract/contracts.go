// Package applicationcontract describes the home-scoped application API.
package applicationcontract

import "github.com/erranliu/SteerLine/internal/product/httpcontract"

const Protocol = "steerline-application/v2"
const WebProtocol = "steerline-web/v1"

type Identity struct {
	Protocol   string                       `json:"protocol"`
	HomeID     string                       `json:"home_id"`
	InstanceID string                       `json:"instance_id"`
	Process    httpcontract.ProcessIdentity `json:"process"`
	Address    string                       `json:"address"`
	Ready      bool                         `json:"ready"`
}

type Project struct {
	RuntimeState string `json:"runtime_state"`
	InstanceID   string `json:"instance_id,omitempty"`
	ConfigID     string `json:"config_id"`
	Name         string `json:"name"`
	Repository   string `json:"repository"`
	ProjectID    string `json:"project_id,omitempty"`
	Online       bool   `json:"online"`
	Conflict     string `json:"conflict,omitempty"`
	Diagnostic   string `json:"diagnostic,omitempty"`
}

// Shutdown is bounded, private lifecycle evidence, never task authority.
type Shutdown struct {
	Version    int               `json:"version"`
	Identity   Identity          `json:"identity"`
	Projects   []ProjectShutdown `json:"projects"`
	Runner     *RunnerShutdown   `json:"runner"`
	Clean      bool              `json:"clean"`
	Diagnostic string            `json:"diagnostic,omitempty"`
}
type ProjectShutdown struct {
	ConfigID      string `json:"config_id"`
	ProjectID     string `json:"project_id,omitempty"`
	InstanceID    string `json:"instance_id,omitempty"`
	LocalClosed   bool   `json:"local_closed"`
	LocksReleased bool   `json:"locks_released"`
	Diagnostic    string `json:"diagnostic,omitempty"`
}
type Catalog struct {
	LoadedExecutionDigest string    `json:"loaded_execution_digest,omitempty"`
	Projects              []Project `json:"projects"`
}

// RunnerShutdown is the single home-wide service settlement, not project state.
type RunnerShutdown struct {
	Attempted        bool   `json:"attempted"`
	StateDir         string `json:"state_dir,omitempty"`
	InstanceID       string `json:"instance_id,omitempty"`
	ExecutableSHA256 string `json:"executable_sha256,omitempty"`
	Version          string `json:"version,omitempty"`
	ResourceDigest   string `json:"resource_digest,omitempty"`
	RequestID        string `json:"request_id,omitempty"`
	Stopped          bool   `json:"stopped"`
	ResourcesSettled bool   `json:"resources_settled"`
	Diagnostic       string `json:"diagnostic,omitempty"`
}

func (r *RunnerShutdown) Settled() bool {
	if r == nil || !r.ResourcesSettled || r.Diagnostic != "" {
		return false
	}
	if !r.Attempted {
		return r.InstanceID == "" && r.RequestID == "" && r.StateDir == ""
	}
	return r.Stopped && r.InstanceID != "" && r.RequestID != "" && r.StateDir != "" && r.ExecutableSHA256 != "" && r.ResourceDigest != "" && r.Version != ""
}
