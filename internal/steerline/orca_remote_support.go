package steerline

import (
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func remoteExecutionRef(run *Run, execution *ExecutionRecord) executionapi.ExecutionRef {
	ref := executionapi.ExecutionRef{Version: executionapi.ExecutionRecordVersion, Backend: executionapi.BackendOrca,
		Protocol: executionapi.ProtocolOrcaSupervised, Agent: executionapi.AgentSpec{Name: execution.Agent},
		RunID: run.RunID, AttemptID: run.AttemptID, Target: execution.Target,
		WorkspacePath: execution.Workspace.Worktree, WorkspaceBranch: execution.Workspace.Branch, BaseCommit: execution.Workspace.Base}
	if execution.Process != nil {
		ref.External = *execution.Process
	}
	return ref
}
