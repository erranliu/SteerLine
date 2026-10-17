package model

import (
	"encoding/json"
	"time"
)

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

// ProjectTaskAuthority binds a project graph to its repository and dispatch authority.
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
