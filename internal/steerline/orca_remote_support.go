package steerline

import (
	"context"
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
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

func remoteClientForExecution(c Config, execution *ExecutionRecord) remote.Client {
	cfg := remoteWorkerConfig(c)
	if state, err := decodeOrcaExecutionState(execution); err == nil && state != nil {
		cfg.ExpectedRemoteRuntime = state.RemoteRuntimeID
		if state.Coordinator.Terminal != "" {
			cfg.Settings.CoordinatorTerminal = state.Coordinator.Terminal
		}
	}
	return remote.New(cfg)
}

func remoteOrcaQuestionReply(ctx context.Context, c Config, run *Run, execution *ExecutionRecord, question, body string) (orcaworker.MutationReceipt, error) {
	state, err := decodeOrcaExecutionState(execution)
	if err != nil || state == nil {
		return orcaworker.MutationReceipt{}, errors.Join(err, errors.New("remote answer has no fixed coordinator"))
	}
	return orcaworker.ReplyQuestion(ctx, orcaWorkerMetaCaller(c), question, body, run.Target.OrchestrationRun,
		state.Coordinator.Terminal, execution.Target.Dispatch, state.Coordinator.RuntimeID)
}
