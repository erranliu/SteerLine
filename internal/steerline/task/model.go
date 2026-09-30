// Package task owns durable task values, validation, and lifecycle rules.
package task

import (
	"encoding/json"
	"time"
)

type TaskRecord struct {
	ProjectIdentity   string     `json:"project_identity"`
	TaskID            string     `json:"task_id"`
	ExternalProvider  string     `json:"external_provider,omitempty"`
	ExternalProject   string     `json:"external_project,omitempty"`
	ExternalID        string     `json:"external_id,omitempty"`
	IdentityMapDigest string     `json:"identity_map_digest,omitempty"`
	BatchID           string     `json:"batch_id,omitempty"`
	Title             string     `json:"title"`
	Type              string     `json:"type,omitempty"`
	Priority          int        `json:"priority,omitempty"`
	GoalIDs           []string   `json:"goal_ids,omitempty"`
	ContractID        string     `json:"contract_id,omitempty"`
	Claimant          string     `json:"claimant,omitempty"`
	Source            string     `json:"source"`
	Status            string     `json:"status"`
	Version           uint64     `json:"version"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	DiscardedAt       *time.Time `json:"discarded_at,omitempty"`
	DiscardReason     string     `json:"discard_reason,omitempty"`
	DiscardedBy       string     `json:"discarded_by,omitempty"`
	DiscardRequestID  string     `json:"discard_request_id,omitempty"`
	CleanupPending    []string   `json:"cleanup_pending,omitempty"`
}

// FileReference pins a contract document to an immutable commit and the
// SHA-256 of the committed blob bytes.
type FileReference struct {
	Path   string `json:"path"`
	Commit string `json:"commit"`
	SHA256 string `json:"sha256"`
}

type TaskContract struct {
	ContractID     string          `json:"contract_id"`
	Repository     string          `json:"repository"`
	Remote         string          `json:"remote"`
	Branch         string          `json:"branch"`
	SpecRef        FileReference   `json:"spec_ref"`
	PlanRef        FileReference   `json:"plan_ref"`
	CommonRefs     []FileReference `json:"common_refs"`
	AcceptanceRefs []FileReference `json:"acceptance_refs"`
	Digest         string          `json:"digest"`
}

type ProjectTaskAuthority struct {
	ProjectIdentity  string    `json:"project_id"`
	Authority        string    `json:"authority"`
	AuthorityVersion uint64    `json:"authority_version"`
	GraphRevision    uint64    `json:"graph_revision"`
	Repository       string    `json:"repository"`
	Remote           string    `json:"remote"`
	Branch           string    `json:"branch"`
	DispatchMode     string    `json:"dispatch_mode"`
	DispatchPaused   bool      `json:"dispatch_paused"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type TaskBatchRecord struct {
	BatchID         string            `json:"batch_id"`
	ProjectIdentity string            `json:"project_id"`
	SchemaVersion   string            `json:"schema_version"`
	PayloadDigest   string            `json:"payload_digest"`
	CreatedAt       time.Time         `json:"created_at"`
	CommandID       string            `json:"command_id"`
	TaskIDs         map[string]string `json:"task_ids"`
}

type TaskDependencyRecord struct {
	ProjectIdentity string `json:"project_id"`
	PredecessorID   string `json:"predecessor_id"`
	SuccessorID     string `json:"successor_id"`
}

type TaskDeliveryRecord struct {
	ProjectIdentity       string          `json:"project_id"`
	TaskID                string          `json:"task_id"`
	SelectedWorkflowRunID string          `json:"selected_workflow_run_id"`
	PRInstance            string          `json:"pr_instance"`
	PRRepository          string          `json:"pr_repository"`
	PRNumber              int64           `json:"pr_number"`
	PRURL                 string          `json:"pr_url"`
	MergeCommit           string          `json:"merge_commit"`
	MergedAt              time.Time       `json:"merged_at"`
	RecordedAt            time.Time       `json:"recorded_at"`
	Evidence              json.RawMessage `json:"evidence,omitempty"`
}

type TaskDeliveryEvidence struct {
	PRInstance         string           `json:"pr_instance"`
	URL                string           `json:"url"`
	Repository         string           `json:"repository"`
	PRNumber           int64            `json:"number"`
	BaseBranch         string           `json:"base_branch"`
	State              string           `json:"state"`
	Merged             bool             `json:"merged"`
	MergedAt           time.Time        `json:"merged_at"`
	MergeCommit        string           `json:"merge_commit"`
	ExistingAcceptance *json.RawMessage `json:"existing_acceptance,omitempty"`
}
