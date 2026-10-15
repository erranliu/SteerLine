// Package applicationcontract describes the home-scoped application API.
package applicationcontract

import "github.com/erranliu/SteerLine/internal/product/httpcontract"

const Protocol = "steerline-application/v1"
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
	ConfigID   string `json:"config_id"`
	Name       string `json:"name"`
	Repository string `json:"repository"`
	ProjectID  string `json:"project_id,omitempty"`
	Online     bool   `json:"online"`
	Conflict   string `json:"conflict,omitempty"`
	Diagnostic string `json:"diagnostic,omitempty"`
}
type Catalog struct {
	Projects []Project `json:"projects"`
}
