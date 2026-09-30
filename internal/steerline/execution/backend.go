// Package execution defines shared execution values and consumer-sized ports.
// It has no business persistence authority and performs no platform I/O.
package execution

import (
	"context"
)

// Backend and protocol are deliberately independent; persisted refs carry both.
type BackendKind string
type ExecutionProtocol string

const (
	BackendLocal                BackendKind       = "local"
	BackendOrca                 BackendKind       = "orca"
	ProtocolLocalProcess        ExecutionProtocol = "local_process"
	ProtocolLocalCodexAppServer ExecutionProtocol = "local_codex_app_server_v1"
	ProtocolOrcaTerminalV2      ExecutionProtocol = "orca_terminal_v2"
	ProtocolOrcaSupervised      ExecutionProtocol = "orca_supervised"
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

type ExecutionProbe interface {
	Identity() (BackendKind, ExecutionProtocol)
	Probe(context.Context, ExecutionTarget, AgentSpec) (CapabilityReport, error)
}

// Observation and workspace consumers do not require cancellation support.
type ExecutionBackend interface {
	ExecutionProbe
	Observe(context.Context, ExecutionRef) (Observation, error)
	InspectWorkspace(context.Context, ExecutionRef) (WorkspaceInspection, error)
}

// StopHandle is the verified native resource handle, never a center Worker.
type StopHandle interface {
	Identity() ProcessIdentity
	Kill()
	Close()
}

type ExecutionStopper interface {
	StopExecution(context.Context, ExecutionRef, Operation, StopHandle) (StopReceipt, error)
}
type ProcessIdentity struct {
	Kind                 string `json:"kind,omitempty"`
	PID                  int    `json:"pid,omitempty"`
	Created              string `json:"created,omitempty"`
	Executable           string `json:"executable,omitempty"`
	Terminal             string `json:"terminal,omitempty"`
	RuntimeID            string `json:"runtime_id,omitempty"`
	Incarnation          string `json:"incarnation,omitempty"`
	CoordinatorRuntimeID string `json:"coordinator_runtime_id,omitempty"`
	OrchestrationRun     string `json:"orchestration_run,omitempty"`
	Task                 string `json:"task,omitempty"`
	Dispatch             string `json:"dispatch,omitempty"`
	ExecutionHost        string `json:"execution_host,omitempty"`
	WorktreeID           string `json:"worktree_id,omitempty"`
}
