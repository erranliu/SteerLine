package model

import (
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type FixedExecutionProfile struct {
	ImplementationVersion string                                        `json:"implementation_version"`
	ImplementationKey     string                                        `json:"implementation_key"`
	ImplementationData    []executionbackend.ImplementationDataEnvelope `json:"implementation_data"`
	ImplementationDisplay []string                                      `json:"implementation_display,omitempty"`
	PublicationPolicy     executionbackend.PublicationPolicy            `json:"publication_policy"`
	Publication           PublicationSettings                           `json:"publication"`
	Version               int                                           `json:"version"`

	AcceptedConfigVersion string `json:"accepted_config_version"`

	ID     string `json:"id"`
	Name   string `json:"name"`
	Digest string `json:"digest"`
	Host   string `json:"host"`
	Agent  string `json:"agent"`
}

// PolicyResources is the resource stage of the same execution snapshot.
// Native configuration verification does not assert successful worker tools.
type PolicyResources struct {
	Version            int                                         `json:"version"`
	Digest             string                                      `json:"digest"`
	ProfileDigest      string                                      `json:"profile_digest"`
	Workspace          string                                      `json:"workspace"`
	WriteRoots         []string                                    `json:"write_roots"`
	TemporaryRoots     []string                                    `json:"temporary_roots"`
	ImplementationData executionbackend.ImplementationDataEnvelope `json:"implementation_data"`
}
