package backend

import (
	"time"
)

type CapabilityStatus string

const (
	CapabilityVerified    CapabilityStatus = "verified"
	CapabilityUnsupported CapabilityStatus = "unsupported"
	CapabilityNotTested   CapabilityStatus = "not_tested"
)

type Capability struct {
	Name     string           `json:"name"`
	Status   CapabilityStatus `json:"status"`
	Evidence string           `json:"evidence,omitempty"`
}

type CapabilityReport struct {
	Version              int               `json:"version"`
	Backend              BackendKind       `json:"backend"`
	Protocol             ExecutionProtocol `json:"protocol"`
	Agent                AgentSpec         `json:"agent"`
	Target               ExecutionTarget   `json:"target"`
	CheckedAt            time.Time         `json:"checked_at"`
	CenterVersion        string            `json:"center_version,omitempty"`
	PlatformVersion      string            `json:"platform_version,omitempty"`
	ExpectedAgentVersion string            `json:"expected_agent_version,omitempty"`
	AgentVersion         string            `json:"agent_version,omitempty"`
	Capabilities         []Capability      `json:"capabilities"`
}
