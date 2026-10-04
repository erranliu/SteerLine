package backend

import (
	"errors"
	"path/filepath"
)

type ProbeIdentity struct {
	Execution     string `json:"execution"`
	Turn          string `json:"turn"`
	Nonce         string `json:"nonce"`
	ProfileDigest string `json:"profile_digest"`
	Runtime       string `json:"runtime"`
	Terminal      string `json:"terminal"`
	Incarnation   string `json:"incarnation"`
	Worktree      string `json:"worktree"`
}

type HostActionKind string

const (
	HostPrepareProbeFiles HostActionKind = "prepare_probe_files"
	HostCleanupProbeFiles HostActionKind = "cleanup_probe_files"
	HostOpenOriginal      HostActionKind = "open_original_host"
	HostRemoveFeedback    HostActionKind = "remove_feedback_request"
)

type HostActionInput struct {
	Kind          HostActionKind           `json:"kind"`
	Probe         *ProbeIdentity           `json:"probe,omitempty"`
	Process       *ProcessIdentity         `json:"process,omitempty"`
	Authorization *HostActionAuthorization `json:"authorization,omitempty"`
	Worktree      string                   `json:"worktree,omitempty"`
	WorkflowRun   string                   `json:"workflow_run_id,omitempty"`
	TurnID        string                   `json:"turn_id,omitempty"`
}

type HostActionAuthorization struct {
	ExecutionID, AttemptID, TurnID, CommandID, BlockerID string
	BlockerVersion                                       uint64
}

func (input HostActionInput) Validate() error {
	valid := false
	switch input.Kind {
	case HostPrepareProbeFiles, HostCleanupProbeFiles:
		valid = input.Probe != nil && input.Probe.Execution != "" && input.Probe.Turn != "" && input.Probe.Nonce != "" &&
			input.Probe.ProfileDigest != "" && input.Probe.Runtime != "" && input.Probe.Terminal != "" && input.Probe.Incarnation != "" && filepath.IsAbs(input.Probe.Worktree) &&
			input.Process == nil && input.Authorization == nil && input.Worktree == "" && input.WorkflowRun == "" && input.TurnID == ""
	case HostOpenOriginal:
		valid = input.Process != nil && input.Process.Kind != "" && input.Process.Terminal != "" && input.Process.RuntimeID != "" && input.Process.Incarnation != "" &&
			input.Authorization != nil && input.Authorization.ExecutionID != "" && input.Authorization.AttemptID != "" && input.Authorization.TurnID != "" &&
			input.Authorization.CommandID != "" && input.Authorization.BlockerID != "" && input.Authorization.BlockerVersion > 0 &&
			input.Probe == nil && input.Worktree == "" && input.WorkflowRun == "" && input.TurnID == ""
	case HostRemoveFeedback:
		valid = input.Probe == nil && input.Process == nil && input.Authorization == nil && filepath.IsAbs(input.Worktree) && input.WorkflowRun != "" && input.TurnID != ""
	}
	if !valid {
		return errors.New("host action input is incomplete or does not match its action")
	}
	return nil
}
