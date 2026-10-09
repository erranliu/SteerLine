package orcaworker

import (
	"context"
	"encoding/json"
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type originalWorkerResource struct {
	Dispatch struct {
		ID   string `json:"id"`
		Task string `json:"taskId"`
		Run  string `json:"runId"`
	} `json:"dispatch"`
	Worker struct {
		Dispatch string `json:"dispatchId"`
		State    string `json:"state"`
	} `json:"worker"`
	Resource struct {
		Terminal     string `json:"terminalHandle"`
		Worktree     string `json:"worktreeId"`
		Owner        string `json:"ownerDispatchId"`
		Origin       string `json:"originDispatchId"`
		ReleaseState string `json:"releaseState"`
	} `json:"terminalResource"`
}

func observeOriginalWorkerResource(ctx context.Context, call MetaCall, identity executionbackend.ProcessIdentity) (originalWorkerResource, json.RawMessage, error) {
	result, err := call(ctx, "orchestration", "worker-show", "--dispatch", identity.Dispatch, "--json")
	var shown originalWorkerResource
	if err != nil {
		return shown, result.Payload, err
	}
	if json.Unmarshal(result.Payload, &shown) != nil || result.RuntimeID != identity.RuntimeID || shown.Dispatch.ID != identity.Dispatch || shown.Dispatch.Task != identity.Task || shown.Dispatch.Run != identity.OrchestrationRun || shown.Worker.Dispatch != identity.Dispatch {
		return shown, result.Payload, errors.New("worker observation changed its original Run/Task/Dispatch/runtime")
	}
	r := shown.Resource
	if r.Terminal == "" || identity.Terminal != "" && r.Terminal != identity.Terminal || r.Worktree != identity.WorktreeID || r.Owner != "" && r.Owner != identity.Dispatch || r.Owner != identity.Dispatch && r.Origin != identity.Dispatch {
		return shown, result.Payload, errors.New("worker resource belongs to another original owner or worktree")
	}
	return shown, result.Payload, nil
}

func originalResourceReleased(ctx context.Context, call MetaCall, identity executionbackend.ProcessIdentity) (bool, json.RawMessage, error) {
	shown, raw, err := observeOriginalWorkerResource(ctx, call, identity)
	return err == nil && shown.Resource.ReleaseState == "released", raw, err
}
func originalWorkerStopped(ctx context.Context, call MetaCall, identity executionbackend.ProcessIdentity) (bool, json.RawMessage, error) {
	shown, raw, err := observeOriginalWorkerResource(ctx, call, identity)
	return err == nil && (shown.Worker.State == "stopped" || shown.Resource.ReleaseState == "released"), raw, err
}

func (l *ExecutionLifecycle) reconcileStop(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionbackend.OperationUnknown)
	if request.Operation.Ref == nil {
		return receipt, nil, errors.New("stop reconciliation lacks its saved original resource binding")
	}
	original := *request.Operation.Ref
	identity := original.External
	if identity.Dispatch == "" {
		state, _, err := l.readState(request.Snapshot)
		if err != nil || state.PartialWorker == nil {
			return receipt, nil, errors.Join(err, errors.New("stop has no original worker resource"))
		}
		if state.PartialWorker.DispatchID != original.Target.Dispatch || state.PartialWorker.TaskID != original.Target.ExternalTask || state.PartialWorker.RunID != original.Target.OrchestrationRun {
			return receipt, nil, errors.New("partial stop changed its original Dispatch binding")
		}
		identity = state.PartialWorker.identity(original.Target.WorktreeID)
	}
	stopped, raw, err := originalWorkerStopped(ctx, l.client.MetaCall, identity)
	receipt.Operation.Receipt = raw
	receipt.RequestID = nativeRequestID(request.Operation.ID)
	if stopped {
		receipt.Disposition = executionbackend.OperationConfirmed
		receipt.State.Stopped = true
	}
	return receipt, nil, err
}
