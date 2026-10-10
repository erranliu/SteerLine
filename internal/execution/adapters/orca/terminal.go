package orcaworker

import "encoding/json"

// Terminal is the Orca terminal identity and status observed by SteerLine.
type Terminal struct {
	Handle          string             `json:"handle"`
	Title           string             `json:"title"`
	WorktreeID      string             `json:"worktreeId"`
	WorktreePath    string             `json:"worktreePath"`
	Branch          string             `json:"branch"`
	Status          string             `json:"status"`
	Connected       bool               `json:"connected"`
	Orphaned        bool               `json:"orphaned"`
	IncarnationID   string             `json:"incarnationId"`
	ExecutionHostID string             `json:"executionHostId"`
	AgentIdentity   string             `json:"agentIdentity"`
	LastOutputAt    int64              `json:"lastOutputAt"`
	AgentWait       *json.RawMessage   `json:"agentWait"`
	ExitCause       *TerminalExitCause `json:"exitCause,omitempty"`
	RuntimeID       string             `json:"-"`
}

type TerminalExitCause struct {
	Kind     string `json:"kind"`
	ExitCode *int   `json:"exitCode,omitempty"`
}

func (t Terminal) Exited() bool {
	return t.Status == "exited" || t.ExitCause != nil && t.ExitCause.Kind == "exited"
}

// An evaluated null wait differs from a field the native host did not report.
func (t *Terminal) UnmarshalJSON(raw []byte) error {
	type plain Terminal
	var value plain
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	if wait, ok := fields["agentWait"]; ok {
		copy := append(json.RawMessage(nil), wait...)
		value.AgentWait = &copy
	}
	*t = Terminal(value)
	return nil
}
