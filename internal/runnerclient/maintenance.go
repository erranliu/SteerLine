package runnerclient

import (
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// ValidateMaintenance checks only the capabilities this caller actually uses.
func (c *Catalog) ValidateMaintenance(write, read runnercontract.ExecutionInput) error {
	if c == nil {
		return errors.New("Runner能力目录不可用")
	}
	r := c.Capabilities.WorktreeReuse
	if r == nil || r.InputProperty != "reuse_worktree_from_run_id" || r.Workflow != "execute/1" || !r.NewSession || r.ArbitraryDirectory {
		return errors.New("Runner缺少受管工作区复用能力")
	}
	local := runnercontract.ExecutionTarget{Kind: "local"}
	if read.Adapter != "codex-app-server" || read.Target != local {
		return errors.New("维护的只读决策需要本机 Codex")
	}
	switch {
	case write.Adapter == "codex-app-server" && write.Target == local:
	case write.Adapter == "orca-terminal" && write.Target.Kind == "orca_environment" && write.Target.Environment != "":
		if !c.hasPairedMaintenanceGit() {
			return errors.New("Runner 缺少远端维护工作区的 Git 取证能力")
		}
	default:
		return errors.New("当前执行组合不支持维护")
	}
	if err := c.validateExecuteFinal(write); err != nil {
		return err
	}
	if err := c.fileMaterialGenerationUnavailable(); err != "" {
		return errors.New(err)
	}
	return c.ValidateConfiguration(read, runnercontract.Workflow{ID: "generate", Version: 1})
}

func (c *Catalog) hasPairedMaintenanceGit() bool {
	var evidence struct {
		Version int `json:"version"`
		Git     struct {
			Command     string `json:"command"`
			ReadOnly    bool   `json:"read_only"`
			AgentCalls  *int   `json:"agent_calls"`
			ResumeCalls *int   `json:"resume_calls"`
			Targets     []struct {
				Adapter    string `json:"adapter"`
				TargetKind string `json:"target_kind"`
				Workflow   string `json:"workflow"`
				Source     string `json:"source"`
				Workspace  string `json:"workspace"`
			} `json:"targets"`
		} `json:"git"`
	}
	if json.Unmarshal(c.Capabilities.RunEvidence, &evidence) != nil || evidence.Version != 1 ||
		evidence.Git.Command != "workspace-git --run-id R" || !evidence.Git.ReadOnly ||
		evidence.Git.AgentCalls == nil || *evidence.Git.AgentCalls != 0 ||
		evidence.Git.ResumeCalls == nil || *evidence.Git.ResumeCalls != 0 {
		return false
	}
	for _, target := range evidence.Git.Targets {
		if target.Adapter == "orca-terminal" && target.TargetKind == "orca_environment" &&
			target.Workflow == "execute/1" && target.Source == "original_authenticated_orca_git_status" &&
			target.Workspace == "latest_managed_worktree_incarnation" {
			return true
		}
	}
	return false
}
