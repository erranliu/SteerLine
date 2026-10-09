package backend

import (
	"time"
)

type ExecutionTarget struct {
	ID                        string `json:"id,omitempty"`
	ConnectionID              string `json:"connection_id,omitempty"`
	WorktreeID                string `json:"worktree_id,omitempty"`
	Branch                    string `json:"branch,omitempty"`
	BaseRevision              string `json:"base_revision,omitempty"`
	ExternalRuntime           string `json:"external_runtime,omitempty"`
	OrchestrationRun          string `json:"orchestration_run,omitempty"`
	OrchestrationRunRequestID string `json:"orchestration_run_request_id,omitempty"`
	CoordinatorTerminal       string `json:"coordinator_terminal,omitempty"`
	ExternalTask              string `json:"external_task,omitempty"`
	Dispatch                  string `json:"dispatch,omitempty"`
	ExecutionHost             string `json:"execution_host,omitempty"`
}

type WorkspaceIntent struct {
	Path       string `json:"path"`
	Branch     string `json:"branch"`
	BaseCommit string `json:"base_commit"`
}

type WorkspaceInspection struct {
	ContentDigest string    `json:"content_digest"`
	Repository    string    `json:"repository"`
	Worktree      string    `json:"worktree"`
	Branch        string    `json:"branch"`
	Base          string    `json:"base"`
	Head          string    `json:"head"`
	Dirty         bool      `json:"dirty"`
	CheckedAt     time.Time `json:"checked_at"`
}
