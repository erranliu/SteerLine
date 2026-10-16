package model

import (
	"github.com/erranliu/SteerLine/internal/execution/agentlaunch"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type FixedExecutionProfile struct {
	ServiceStopBehavior   executionbackend.ServiceStopBehavior          `json:"service_stop_behavior"`
	ImplementationVersion string                                        `json:"implementation_version"`
	ImplementationKey     string                                        `json:"implementation_key"`
	ImplementationData    []executionbackend.ImplementationDataEnvelope `json:"implementation_data"`
	ImplementationDisplay []string                                      `json:"implementation_display,omitempty"`
	PublicationPolicy     executionbackend.PublicationPolicy            `json:"publication_policy"`
	Publication           PublicationSettings                           `json:"publication"`
	Version               int                                           `json:"version"`

	AcceptedConfigVersion string `json:"accepted_config_version"`

	ID                 string              `json:"id"`
	Name               string              `json:"name"`
	Digest             string              `json:"digest"`
	Host               string              `json:"host"`
	Agent              string              `json:"agent"`
	AgentLaunch        *agentlaunch.Config `json:"agent_launch,omitempty"`
	AgentLaunchCommand string              `json:"agent_launch_command,omitempty"`
}
