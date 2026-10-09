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
	// A not-yet-observed turn can already own accepted input. Keep its original
	// resource and uncertainty; the observer/operator must inspect it, not cleanup.
	cleanup := started.State != "outcome_unknown" || started.Stage != "turn_start_unobserved"
	return l.failedStartReceipt(ctx, request, state, started, result, reason, cleanup)
}

func (l *ExecutionLifecycle) reconcileFailedStart(ctx context.Context, request executionbackend.LifecycleRequest, state NativeExecutionState, started workerStartReceipt, result Result, reason error) (executionbackend.LifecycleReceipt, error) {
	return l.failedStartReceipt(ctx, request, state, started, result, reason, false)
}

func (l *ExecutionLifecycle) failedStartReceipt(ctx context.Context, request executionbackend.LifecycleRequest, state NativeExecutionState, started workerStartReceipt, result Result, reason error, cleanup bool) (executionbackend.LifecycleReceipt, error) {
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
	shown, proof, observeErr := observeOriginalWorkerResource(ctx, l.client.MetaCall, identity)
	if observeErr != nil {
		receipt.Diagnostic += "; partial resource ownership is unverified: " + observeErr.Error()
		receipt.Operation.Receipt, _ = json.Marshal(map[string]json.RawMessage{"start": result.Payload, "resource_observation": proof})
		receipt.Implementation, observeErr = l.targetEnvelope(request.Snapshot, target, request.Snapshot.Ref.External, state)
		return receipt, errors.Join(reason, observeErr)
	}
	if shown.Resource.ReleaseState == "released" {
		state.PartialWorker = nil
		state.FailedStartReleased = true
		receipt.State.Stopped, receipt.State.WorkerAbsent, receipt.State.WritersAbsent = true, true, true
		receipt.Disposition = executionbackend.OperationRejected
		receipt.Operation.Receipt, _ = json.Marshal(map[string]json.RawMessage{"start": result.Payload, "release_observation": proof})
		receipt.Diagnostic += "; original partial worker release proven by persistent Orca resource"
		receipt.Implementation, observeErr = l.targetEnvelope(request.Snapshot, target, request.Snapshot.Ref.External, state)
		return receipt, errors.Join(reason, observeErr)
	}
	if !cleanup {
		receipt.Operation.Receipt, _ = json.Marshal(map[string]json.RawMessage{"start": result.Payload, "resource_observation": proof})
		receipt.Implementation, observeErr = l.targetEnvelope(request.Snapshot, target, request.Snapshot.Ref.External, state)
		return receipt, errors.Join(reason, observeErr)
	}
	effects := map[string]json.RawMessage{"start": result.Payload, "resource_observation": proof}
	stopped := shown.Worker.State == "stopped"
	if (started.State == "outcome_unknown" || result.CallUncertain) && !stopped {
		// Unobserved input is not positive evidence of process exit. Preserve
		// this exact resource for observation rather than closing its terminal.
		receipt.Operation.Receipt, _ = json.Marshal(effects)
		receipt.Implementation, observeErr = l.targetEnvelope(request.Snapshot, target, request.Snapshot.Ref.External, state)
		return receipt, errors.Join(reason, observeErr)
	}
	if !stopped {
		stop, stopProof, stopErr := workerResourceAction(ctx, l.client.MetaCall, identity, "worker-stop", request.Operation.ID+":failed-start-stop")
		effects["stop"] = stop.Payload
		stopped = stopErr == nil && (stopProof.State == "stopped" || stopProof.AlreadySettled)
		if !stopped {
			proven, raw, err := originalWorkerStopped(ctx, l.client.MetaCall, identity)
			effects["stop_observation"] = raw
			stopped = err == nil && proven
		}
	}
	if stopped {
		released, releaseProof, releaseErr := workerResourceAction(ctx, l.client.MetaCall, identity, "worker-release", request.Operation.ID+":failed-start-release")
		effects["release"] = released.Payload
		if releaseErr == nil && workerReleased(releaseProof) {
			state.PartialWorker = nil
			state.FailedStartReleased = true
			receipt.State.Stopped, receipt.State.WorkerAbsent, receipt.State.WritersAbsent = true, true, true
			receipt.Disposition = executionbackend.OperationRejected
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
