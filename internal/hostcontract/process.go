package hostcontract

// ProcessProbeFacts is the small OS observation returned to application hosts.
type ProcessProbeFacts struct {
	State  string
	Reason string
}

// ProcessFactsPort owns process identity and liveness observations. It does
// not start or stop processes and does not carry execution state.
type ProcessFactsPort interface {
	CurrentIdentity() (*ProcessIdentity, error)
	IdentityForPID(int) (ProcessIdentity, error)
	IdentityFromHandle(uintptr, int) (ProcessIdentity, error)
	SessionID(int) (int, error)
	Probe(*ProcessIdentity) ProcessProbeFacts
}

type ProcessIdentity struct {
	Kind          string `json:"kind,omitempty"`
	PID           int    `json:"pid,omitempty"`
	Created       string `json:"created,omitempty"`
	Executable    string `json:"executable,omitempty"`
	Terminal      string `json:"terminal,omitempty"`
	RuntimeID     string `json:"runtime_id,omitempty"`
	Incarnation   string `json:"incarnation,omitempty"`
	ExecutionHost string `json:"execution_host,omitempty"`
	WorktreeID    string `json:"worktree_id,omitempty"`
}
