package orcaworker

import (
	"context"
	"encoding/json"
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type partialWorker struct {
	RunID      string `json:"run_id"`
	TaskID     string `json:"task_id"`
	DispatchID string `json:"dispatch_id"`
	RuntimeID  string `json:"runtime_id"`
}

// A failed composite start can already have injected input. Preserve and
// recover its exact native resource, without inventing a terminal or session.
func (l *ExecutionLifecycle) recoverFailedStart(ctx context.Context, request executionbackend.LifecycleRequest, state NativeExecutionState, started workerStartReceipt, result Result, reason error) (executionbackend.LifecycleReceipt, error) {
	receipt := nativeReceipt(request, executionbackend.OperationUnknown)
	receipt.RequestID = nativeRequestID(request.Operation.ID)
	receipt.Diagnostic = reason.Error()
	receipt.Operation.Receipt = append([]byte(nil), result.Payload...)
	turn := nativeTurn(&state, request.Input.Turn.CommandID)
	if turn == nil || started.RunID != request.Snapshot.Ref.Target.OrchestrationRun || started.TaskID != turn.TaskID || started.DispatchID == "" || result.RuntimeID != state.Coordinator.RuntimeID {
		return receipt, reason
	}
	partial := partialWorker{RunID: started.RunID, TaskID: started.TaskID, DispatchID: started.DispatchID, RuntimeID: result.RuntimeID}
	state.PartialWorker = &partial
	target := request.Snapshot.Ref.Target
	target.Dispatch, target.ExternalTask = partial.DispatchID, partial.TaskID
	receipt.Target = &target
	identity := partial.identity(target.WorktreeID)
	if released, proof, observeErr := originalResourceReleased(ctx, l.client.MetaCall, identity); observeErr == nil && released {
		state.PartialWorker = nil
		state.FailedStartReleased = true
		receipt.State.Stopped, receipt.State.WorkerAbsent, receipt.State.WritersAbsent = true, true, true
		receipt.Operation.Receipt, _ = json.Marshal(map[string]json.RawMessage{"start": result.Payload, "release_observation": proof})
		receipt.Diagnostic += "; original partial worker release proven by persistent Orca resource"
		receipt.Implementation, observeErr = l.targetEnvelope(request.Snapshot, target, request.Snapshot.Ref.External, state)
		return receipt, errors.Join(reason, observeErr)
	}
	stop, stopProof, stopErr := workerResourceAction(ctx, l.client.MetaCall, identity, "worker-stop", request.Operation.ID+":failed-start-stop")
	effects := map[string]json.RawMessage{"start": result.Payload, "stop": stop.Payload}
	if stopErr == nil && (stopProof.State == "stopped" || stopProof.AlreadySettled) {
		released, releaseProof, releaseErr := workerResourceAction(ctx, l.client.MetaCall, identity, "worker-release", request.Operation.ID+":failed-start-release")
		effects["release"] = released.Payload
		if releaseErr == nil && workerReleased(releaseProof) {
			state.PartialWorker = nil
			state.FailedStartReleased = true
			receipt.State.Stopped, receipt.State.WorkerAbsent, receipt.State.WritersAbsent = true, true, true
			receipt.Diagnostic += "; original partial worker stopped and released; first-turn effects remain recorded"
		} else {
			receipt.Diagnostic += "; original partial worker release is unverified"
		}
	} else {
		receipt.Diagnostic += "; original partial worker stop is unverified"
	}
	raw, _ := json.Marshal(effects)
	receipt.Operation.Receipt = raw
	var bindErr error
	receipt.Implementation, bindErr = l.targetEnvelope(request.Snapshot, target, request.Snapshot.Ref.External, state)
	return receipt, errors.Join(reason, bindErr)
}

func (p partialWorker) identity(worktree string) executionbackend.ProcessIdentity {
	return executionbackend.ProcessIdentity{Kind: "orca", RuntimeID: p.RuntimeID, OrchestrationRun: p.RunID, Task: p.TaskID, Dispatch: p.DispatchID, WorktreeID: worktree}
}
