package orcaworker

import (
	"context"
	"encoding/json"
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func originalResourceReleased(ctx context.Context, call MetaCall, identity executionbackend.ProcessIdentity) (bool, json.RawMessage, error) {
	result, err := call(ctx, "orchestration", "worker-show", "--dispatch", identity.Dispatch, "--json")
	if err != nil {
		return false, result.Payload, err
	}
	var shown struct {
		Dispatch struct {
			ID     string `json:"id"`
			TaskID string `json:"taskId"`
			RunID  string `json:"runId"`
		} `json:"dispatch"`
		Worker struct {
			DispatchID string `json:"dispatchId"`
		} `json:"worker"`
		Resource struct {
			Terminal     string `json:"terminalHandle"`
			Worktree     string `json:"worktreeId"`
			Owner        string `json:"ownerDispatchId"`
			Origin       string `json:"originDispatchId"`
			ReleaseState string `json:"releaseState"`
		} `json:"terminalResource"`
	}
	if json.Unmarshal(result.Payload, &shown) != nil || result.RuntimeID != identity.RuntimeID || shown.Dispatch.ID != identity.Dispatch || shown.Dispatch.TaskID != identity.Task || shown.Dispatch.RunID != identity.OrchestrationRun || shown.Worker.DispatchID != identity.Dispatch {
		return false, result.Payload, errors.New("current resource observation differs from the original Dispatch")
	}
	resource := shown.Resource
	if resource.ReleaseState != "released" {
		return false, result.Payload, nil
	}
	if resource.Terminal == "" || identity.Terminal != "" && resource.Terminal != identity.Terminal || resource.Worktree != identity.WorktreeID || resource.Owner != "" && resource.Owner != identity.Dispatch || resource.Origin != identity.Dispatch && resource.Owner != identity.Dispatch {
		return false, result.Payload, errors.New("released resource belongs to another original worker owner")
	}
	return true, result.Payload, nil
}

func originalWorkerStopped(ctx context.Context, call MetaCall, identity executionbackend.ProcessIdentity) (bool, json.RawMessage, error) {
	result, err := call(ctx, "orchestration", "worker-show", "--dispatch", identity.Dispatch, "--json")
	if err != nil {
		return false, result.Payload, err
	}
	var shown struct {
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
			Terminal string `json:"terminalHandle"`
			Worktree string `json:"worktreeId"`
			Owner    string `json:"ownerDispatchId"`
			Origin   string `json:"originDispatchId"`
		} `json:"terminalResource"`
	}
	if json.Unmarshal(result.Payload, &shown) != nil || result.RuntimeID != identity.RuntimeID || shown.Dispatch.ID != identity.Dispatch || shown.Dispatch.Task != identity.Task || shown.Dispatch.Run != identity.OrchestrationRun || shown.Worker.Dispatch != identity.Dispatch || shown.Resource.Terminal == "" || identity.Terminal != "" && shown.Resource.Terminal != identity.Terminal || shown.Resource.Worktree != identity.WorktreeID || shown.Resource.Owner != "" && shown.Resource.Owner != identity.Dispatch || shown.Resource.Owner != identity.Dispatch && shown.Resource.Origin != identity.Dispatch {
		return false, result.Payload, errors.New("stopped worker observation changed its original resource identity")
	}
	return shown.Worker.State == "stopped", result.Payload, nil
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
