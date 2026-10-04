// Package model owns fixed task values, contracts, DAG and task eligibility.
// Application callers supply task facts and commit accepted changes.
package model

import (
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
