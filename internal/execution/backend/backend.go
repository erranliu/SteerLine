// Package backend retains product display values and SteerLine process ports.
// Native execution and lifecycle ports belong to the standalone SteerRunner.
package backend

import (
	"context"
	"os/exec"
)

// Backend and protocol are deliberately independent; persisted refs carry both.
type BackendKind string

type ExecutionProtocol string

const (
	BackendLocal                BackendKind       = "local"
	BackendOrca                 BackendKind       = "orca"
	ProtocolLocalProcess        ExecutionProtocol = "local_process"
	ProtocolLocalCodexAppServer ExecutionProtocol = "local_codex_app_server_v1"
	ProtocolOrcaTerminalV3      ExecutionProtocol = "orca_terminal_v3"
	ExecutionRecordVersion                        = 1
)

type AgentSpec struct {
	Name string `json:"name"`
}

type ExecutionRef struct {
	Version         int               `json:"version"`
	Backend         BackendKind       `json:"backend"`
	Protocol        ExecutionProtocol `json:"protocol"`
	Target          ExecutionTarget   `json:"target"`
	Agent           AgentSpec         `json:"agent"`
	RunID           string            `json:"run_id"`
	AttemptID       string            `json:"attempt_id"`
	WorkspacePath   string            `json:"workspace_path,omitempty"`
	WorkspaceBranch string            `json:"workspace_branch,omitempty"`
	BaseCommit      string            `json:"base_commit,omitempty"`
	External        ProcessIdentity   `json:"external"`
	Generation      string            `json:"generation,omitempty"`
	Incomplete      bool              `json:"incomplete,omitempty"`
}

// StopHandle is the verified native resource handle, never a center Worker.
type StopHandle interface {
	Identity() ProcessIdentity
	Kill()
	Close()
}

// ManagedProcessHost is a small host-owned process boundary. Implementations
// receive a fixed command constructed by the application; they never
// receive a Service, Run, or persistence handle.
type ManagedProcessHost interface {
	StartManaged(*exec.Cmd) (StopHandle, error)
	InspectProcess(*ProcessIdentity) string
	NewProcessGeneration() string
	WriteExecutionFile(string, []byte) error
	OutputCommand(context.Context, LaunchCommand) ([]byte, error)
}

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
