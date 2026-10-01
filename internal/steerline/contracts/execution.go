package contracts

import (
	"time"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

type Command struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
}

type LegacyCompletionConfig struct {
	Command Command `json:"command"`
	Timeout string  `json:"timeout"`
}

type ControlReceipt struct {
	Version         int        `json:"version"`
	RequestID       string     `json:"request_id"`
	RequestDigest   string     `json:"request_digest"`
	InstanceID      string     `json:"instance_id"`
	Action          string     `json:"action"`
	Status          string     `json:"status"`
	Code            string     `json:"code"`
	Message         string     `json:"message"`
	RequestedBy     string     `json:"requested_by"`
	SubmittedAt     time.Time  `json:"submitted_at"`
	AcceptedAt      *time.Time `json:"accepted_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	RunID           string     `json:"run_id,omitempty"`
	AttemptID       string     `json:"attempt_id,omitempty"`
	DecisionID      string     `json:"decision_id,omitempty"`
	ControlRevision uint64     `json:"control_revision,omitempty"`
	RunRevision     uint64     `json:"run_revision,omitempty"`
}

type FixedExecutionProfile struct {
	ExecutionRequirements      executionapi.ExecutionRequirements        `json:"execution_requirements"`
	RequirementsEvaluation     *executionapi.RequirementsEvaluation      `json:"requirements_evaluation,omitempty"`
	RequirementsEvidenceDigest string                                    `json:"requirements_evidence_digest,omitempty"`
	ImplementationVersion      string                                    `json:"implementation_version"`
	ImplementationKey          string                                    `json:"implementation_key"`
	ImplementationData         []executionapi.ImplementationDataEnvelope `json:"implementation_data"`
	ImplementationDisplay      []string                                  `json:"implementation_display,omitempty"`
	PublicationPolicy          executionapi.PublicationPolicy            `json:"publication_policy"`
	Publication                PublicationSettings                       `json:"publication"`
	Version                    int                                       `json:"version"`
	ExecutionPolicy            executionapi.ExecutionPolicy              `json:"execution_policy"`
	PolicyDigest               string                                    `json:"policy_digest"`
	AcceptedConfigVersion      string                                    `json:"accepted_config_version"`
	PolicySource               string                                    `json:"policy_source"`
	ID                         string                                    `json:"id"`
	Name                       string                                    `json:"name"`
	Digest                     string                                    `json:"digest"`
	Host                       string                                    `json:"host"`
	Agent                      string                                    `json:"agent"`
}

// PolicyResources is the resource stage of the same execution snapshot.
// Native configuration verification does not assert successful worker tools.
type PolicyResources struct {
	Version            int                                     `json:"version"`
	Digest             string                                  `json:"digest"`
	ProfileDigest      string                                  `json:"profile_digest"`
	Workspace          string                                  `json:"workspace"`
	WriteRoots         []string                                `json:"write_roots"`
	TemporaryRoots     []string                                `json:"temporary_roots"`
	ImplementationData executionapi.ImplementationDataEnvelope `json:"implementation_data"`
}
