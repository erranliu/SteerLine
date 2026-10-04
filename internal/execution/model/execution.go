package model

import (
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type FixedExecutionProfile struct {
	ExecutionRequirements      executionbackend.ExecutionRequirements        `json:"execution_requirements"`
	RequirementsEvaluation     *executionbackend.RequirementsEvaluation      `json:"requirements_evaluation,omitempty"`
	RequirementsEvidenceDigest string                                        `json:"requirements_evidence_digest,omitempty"`
	ImplementationVersion      string                                        `json:"implementation_version"`
	ImplementationKey          string                                        `json:"implementation_key"`
	ImplementationData         []executionbackend.ImplementationDataEnvelope `json:"implementation_data"`
	ImplementationDisplay      []string                                      `json:"implementation_display,omitempty"`
	PublicationPolicy          executionbackend.PublicationPolicy            `json:"publication_policy"`
	Publication                PublicationSettings                           `json:"publication"`
	Version                    int                                           `json:"version"`
	ExecutionPolicy            executionbackend.ExecutionPolicy              `json:"execution_policy"`
	PolicyDigest               string                                        `json:"policy_digest"`
	AcceptedConfigVersion      string                                        `json:"accepted_config_version"`
	PolicySource               string                                        `json:"policy_source"`
	ID                         string                                        `json:"id"`
	Name                       string                                        `json:"name"`
	Digest                     string                                        `json:"digest"`
	Host                       string                                        `json:"host"`
	Agent                      string                                        `json:"agent"`
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
