package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type workerStartReceipt struct {
	RunID      string `json:"runId"`
	TaskID     string `json:"taskId"`
	DispatchID string `json:"dispatchId"`
	State      string `json:"state"`
	Stage      string `json:"stage"`
	TurnStart  string `json:"turnStart"`
	Mode       struct {
		Mode string `json:"mode"`
	} `json:"mode"`
	Launch struct {
		Effective struct {
			Agent  string `json:"agent"`
			Model  string `json:"model"`
			Effort string `json:"effort"`
		} `json:"effective"`
	} `json:"launch"`
	Prompt struct {
		RequestID string   `json:"requestId"`
		Stages    []string `json:"stages"`
	} `json:"prompt"`
	Effects []struct {
		Kind string `json:"kind"`
		Role string `json:"role"`
		ID   string `json:"id"`
	} `json:"effects"`
}

func (l *ExecutionLifecycle) startNativeWorker(ctx context.Context, snapshot executionbackend.LifecycleSnapshot, state NativeExecutionState, turn NativeTurnState, operationID string) (Result, workerStartReceipt, executionbackend.ProcessIdentity, error) {
	args := []string{"orchestration", "worker-start", "--task", turn.TaskID, "--worktree", "id:" + snapshot.Ref.Target.WorktreeID, "--run", snapshot.Ref.Target.OrchestrationRun, "--from", state.Coordinator.Terminal, "--retry-request", nativeRequestID(operationID), "--timeout-ms", strconv.FormatInt(positiveDuration(l.orca.WaitTimeout, 2*time.Minute).Milliseconds(), 10), "--json"}
	previous := snapshot.Ref.External
	first := previous.Terminal == ""
	if first {
		args = append(args, "--agent", snapshot.Ref.Agent.Name)
		if l.orca.Model != "" {
			args = append(args, "--model", l.orca.Model)
		}
		if l.orca.ReasoningEffort != "" {
			args = append(args, "--effort", l.orca.ReasoningEffort)
		}
	} else {
		if err := l.verifyAgent(ctx, snapshot, previous); err != nil {
			return Result{}, workerStartReceipt{}, previous, err
		}
		if previous.Dispatch != "" {
			status, err := VerifySettledDispatch(ctx, l.client.MetaCall, DispatchIdentity{RunID: previous.OrchestrationRun, TaskID: previous.Task, DispatchID: previous.Dispatch, RuntimeID: previous.RuntimeID, Terminal: previous.Terminal, Incarnation: previous.Incarnation, WorktreeID: previous.WorktreeID})
			if err != nil || status != "completed" && status != "failed" {
				return Result{}, workerStartReceipt{}, previous, errors.Join(err, errors.New("original worker is not settled for continuation"))
			}
		}
		args = append(args, "--terminal", previous.Terminal)
	}
	wait := positiveDuration(l.orca.WaitTimeout, 2*time.Minute)
	timeout := max(positiveDuration(l.client.config.CommandTimeout, 30*time.Second), wait)
	if wait <= time.Duration(1<<63-1)-30*time.Second {
		timeout = max(timeout, wait+30*time.Second)
	}
	result, callErr := l.client.metaCallTimeout(ctx, timeout, args...)
	var started workerStartReceipt
	parseErr := json.Unmarshal(result.Payload, &started)
	// A durable input receipt may report an unobserved turn with a nonzero CLI
	// exit. It is accepted input, not proof of a failed or dead worker.
	acceptedUnobserved := started.State == "outcome_unknown" && started.Stage == "turn_start_unobserved" &&
		started.TurnStart == "unobserved" && started.Prompt.RequestID == nativeRequestID(operationID) &&
		slices.Contains(started.Prompt.Stages, "input_accepted")
	if parseErr != nil || callErr != nil && (!acceptedUnobserved || result.CallUncertain) {
		cause := errors.Join(callErr, parseErr)
		if parseErr == nil && result.RuntimeID == state.Coordinator.RuntimeID && started.RunID == snapshot.Ref.Target.OrchestrationRun && started.TaskID == turn.TaskID && started.DispatchID != "" {
			cause = fmt.Errorf("original Orca worker-start returned state=%q stage=%q: %w", started.State, started.Stage, cause)
		}
		return result, started, previous, &CallError{Cause: cause, Result: result}
	}
	if result.RuntimeID != state.Coordinator.RuntimeID || started.RunID != snapshot.Ref.Target.OrchestrationRun || started.TaskID != turn.TaskID || started.DispatchID == "" {
		return result, started, previous, errors.New("Orca worker-start receipt changed its original Run/Task/runtime")
	}
	if !acceptedUnobserved && (started.State != "ready" || started.Stage != "input_accepted") {
		return result, started, previous, errors.New("original Orca worker input acceptance is unverified; reconcile the saved request")
	}
	if started.Mode.Mode != "terminal" {
		return result, started, previous, errors.New("Orca worker has no supported original-terminal continuation; preserve first-turn effects and use Orca Terminal mode")
	}
	if first && (started.Launch.Effective.Agent != snapshot.Ref.Agent.Name || l.orca.Model != "" && started.Launch.Effective.Model != l.orca.Model || l.orca.ReasoningEffort != "" && started.Launch.Effective.Effort != l.orca.ReasoningEffort) {
		return result, started, previous, errors.New("Orca native launch differs from the requested agent/model/effort")
	}
	envelope, err := l.client.MetaCall(ctx, "orchestration", "worker-show", "--dispatch", started.DispatchID, "--json")
	if err != nil {
		return result, started, previous, err
	}
	var shown struct {
		Dispatch struct {
			ID     string `json:"id"`
			RunID  string `json:"runId"`
			TaskID string `json:"taskId"`
		} `json:"dispatch"`
		Worker struct {
			DispatchID    string `json:"dispatchId"`
			AgentTerminal string `json:"agentTerminalHandle"`
		} `json:"worker"`
		Terminal *Terminal `json:"terminal"`
	}
	if json.Unmarshal(envelope.Payload, &shown) != nil || envelope.RuntimeID != result.RuntimeID || shown.Dispatch.ID != started.DispatchID || shown.Dispatch.RunID != started.RunID || shown.Dispatch.TaskID != started.TaskID || shown.Worker.DispatchID != started.DispatchID || shown.Terminal == nil || shown.Terminal.Handle == "" || shown.Terminal.Handle != shown.Worker.AgentTerminal {
		return result, started, previous, errors.New("native worker-start has no exact original terminal receipt")
	}
	terminal := *shown.Terminal
	terminal.RuntimeID = envelope.RuntimeID
	if err := ValidateWorkspaceTerminal(terminal, snapshot.Workspace.Path, snapshot.Workspace.Branch); err != nil {
		return result, started, previous, err
	}
	if terminal.AgentIdentity != snapshot.Ref.Agent.Name || terminal.WorktreeID != snapshot.Ref.Target.WorktreeID {
		return result, started, previous, errors.New("native worker-start selected a different agent or worktree")
	}
	process := TerminalProcess(terminal, l.orca.Executable)
	process.WorktreeID, process.OrchestrationRun, process.Task, process.Dispatch = terminal.WorktreeID, started.RunID, started.TaskID, started.DispatchID
	if !first && (process.Terminal != previous.Terminal || process.RuntimeID != previous.RuntimeID || process.Incarnation != previous.Incarnation) {
		return result, started, process, errors.New("continuation replaced the original worker session")
	}
	return result, started, process, nil
}
