package steerline

import "time"

type Repository struct {
	Path     string `json:"path"`
	Remote   string `json:"remote"`
	Branch   string `json:"branch"`
	Identity string `json:"identity"`
}
type Runtime struct {
	SchemaVersion int          `json:"schema_version"`
	UpdatedAt     time.Time    `json:"updated_at"`
	HeartbeatAt   time.Time    `json:"heartbeat_at"`
	Service       ServiceState `json:"service"`
	Repository    Repository   `json:"repository"`
	CurrentTask   *string      `json:"current_task"`
	SlotOccupied  bool         `json:"slot_occupied"`
	Runs          []*Run       `json:"runs"`
}
type ServiceState struct {
	Mode       string  `json:"mode"`
	InstanceID string  `json:"instance_id"`
	PID        int     `json:"pid"`
	Status     string  `json:"status"`
	Claimant   string  `json:"claimant"`
	Error      *string `json:"error"`
}

// ProcessIdentity is the host-neutral identity of a running worker. OS hosts use
// PID/Created/Executable; the Orca host uses Terminal/RuntimeID/Incarnation.
// Kind is "os" for local runs and "orca" when hosted by Orca.
type ProcessIdentity struct {
	Kind        string `json:"kind,omitempty"`
	PID         int    `json:"pid,omitempty"`
	Created     string `json:"created,omitempty"`
	Executable  string `json:"executable,omitempty"`
	Terminal    string `json:"terminal,omitempty"`
	RuntimeID   string `json:"runtime_id,omitempty"`
	Incarnation string `json:"incarnation,omitempty"`
}
type Run struct {
	TaskID           string            `json:"task_id"`
	Title            string            `json:"title"`
	RunID            string            `json:"run_id"`
	Claimant         string            `json:"claimant"`
	Host             string            `json:"host,omitempty"`
	BeadsStatus      string            `json:"beads_status"`
	ExecutionStatus  string            `json:"execution_status"`
	FinishTaskStatus string            `json:"finish_task_status"`
	SessionID        *string           `json:"codex_session_id"`
	Process          *ProcessIdentity  `json:"process"`
	StartedAt        *time.Time        `json:"started_at"`
	EndedAt          *time.Time        `json:"ended_at"`
	WorktreePath     *string           `json:"worktree_path"`
	WorktreeBranch   *string           `json:"worktree_branch"`
	WorkspaceIntent  *WorkspaceIntent  `json:"workspace_intent"`
	Completion       *CompletionConfig `json:"completion"`
	BeadsContext     *BeadsContext     `json:"beads_context"`
	BaseCommit       *string           `json:"base_commit"`
	PR               PRState           `json:"pr"`
	LastEvent        *string           `json:"last_event"`
	LogPath          *string           `json:"log_path"`
	ResultPath       *string           `json:"result_path"`
	Summary          *string           `json:"summary"`
	Error            *string           `json:"error"`
}

// Intent is durable before Git creates anything; actual workspace fields stay null
// until creation and verification succeed.
type WorkspaceIntent struct {
	Path       string `json:"path"`
	Branch     string `json:"branch"`
	BaseCommit string `json:"base_commit"`
}
type PRState struct {
	URL         *string    `json:"url"`
	State       string     `json:"state"`
	MergedAt    *time.Time `json:"merged_at"`
	MergeCommit *string    `json:"merge_commit"`
	QueriedAt   *time.Time `json:"queried_at"`
	QueryError  *string    `json:"query_error"`
}
type Result struct {
	TaskID       string  `json:"task_id"`
	RunID        string  `json:"run_id"`
	Outcome      string  `json:"outcome"`
	PRURL        *string `json:"pr_url"`
	WorktreePath *string `json:"worktree_path"`
	BaseCommit   *string `json:"base_commit"`
	Summary      *string `json:"summary"`
	Error        *string `json:"error"`
}

func ptr[T any](v T) *T { return &v }
func now() time.Time    { return time.Now().UTC() }
