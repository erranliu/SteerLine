package backend

type ExecutionTarget struct {
	ID              string `json:"id,omitempty"`
	ConnectionID    string `json:"connection_id,omitempty"`
	WorktreeID      string `json:"worktree_id,omitempty"`
	Branch          string `json:"branch,omitempty"`
	BaseRevision    string `json:"base_revision,omitempty"`
	ExternalRuntime string `json:"external_runtime,omitempty"`
	ExecutionHost   string `json:"execution_host,omitempty"`
}
