package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// Orca accepts caller-owned mutation UUIDs. Derive one from the already-saved
// center operation so a lost reply never creates a fresh mutation on recovery.
func nativeRequestID(operationID string) string {
	if operationID == "" {
		return ""
	}
	value := digestValue(operationID)[:32]
	value = value[:12] + "4" + value[13:16] + "8" + value[17:]
	return strings.Join([]string{value[:8], value[8:12], value[12:16], value[16:20], value[20:]}, "-")
}

type workerResourceReceipt struct {
	DispatchID     string          `json:"dispatchId"`
	State          string          `json:"state"`
	ProcessAction  string          `json:"processAction"`
	AlreadySettled bool            `json:"alreadySettled"`
	LastError      string          `json:"lastError,omitempty"`
	Reason         string          `json:"reason,omitempty"`
	Recovery       json.RawMessage `json:"recovery,omitempty"`
}

func workerResourceAction(ctx context.Context, call MetaCall, identity executionbackend.ProcessIdentity, action, operationID string) (Result, workerResourceReceipt, error) {
	if identity.Dispatch == "" || identity.Task == "" || identity.OrchestrationRun == "" || identity.RuntimeID == "" || operationID == "" {
		return Result{}, workerResourceReceipt{}, errors.New("worker resource action lacks its original Dispatch/mutation identity")
	}
	result, err := call(ctx, "orchestration", action, "--dispatch", identity.Dispatch, "--retry-request", nativeRequestID(operationID), "--json")
	var proof workerResourceReceipt
	parseErr := json.Unmarshal(result.Payload, &proof)
	if err != nil || parseErr != nil {
		return result, proof, &executionbackend.BackendError{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: "original Orca worker mutation outcome is unverified", Cause: &CallError{Cause: errors.Join(err, parseErr), Result: result}}
	}
	if result.RuntimeID != identity.RuntimeID || proof.DispatchID != identity.Dispatch {
		return result, proof, errors.New("Orca resource receipt belongs to another original Dispatch/runtime")
	}
	return result, proof, nil
}

func workerReleased(proof workerResourceReceipt) bool {
	return proof.State == "released" || proof.State == "already_released"
}

func (l *ExecutionLifecycle) releaseOriginalWorker(ctx context.Context, request executionbackend.LifecycleRequest, state NativeExecutionState) (executionbackend.LifecycleReceipt, error) {
	receipt := nativeReceipt(request, executionbackend.OperationUnknown)
	original := request.Snapshot.Ref.External
	if len(request.Input.Cleanup.Resources) != 1 || request.Input.Cleanup.Resources[0] != original {
		return nativeReceipt(request, executionbackend.OperationRejected), errors.New("worker release differs from the original resource")
	}
	if turn := latestBusinessTurn(&state, request.Snapshot); turn != nil {
		if err := l.freezeOriginalOutput(ctx, request.Snapshot, *turn); err != nil {
			receipt.Diagnostic = "canonical output unavailable: " + err.Error()
		}
	}
	result, proof, err := workerResourceAction(ctx, l.client.MetaCall, original, "worker-release", request.Operation.ID)
	receipt.Operation.Receipt = append([]byte(nil), result.Payload...)
	receipt.RequestID = nativeRequestID(request.Operation.ID)
	if err != nil {
		return receipt, err
	}
	if !workerReleased(proof) {
		receipt.Diagnostic = "Orca worker release remains " + proof.State + "; " + proof.Reason + "; " + proof.LastError + "; recovery=" + string(proof.Recovery)
		return receipt, nil
	}
	state.AgentClosed = true
	receipt.Disposition = executionbackend.OperationConfirmed
	receipt.State.ResourceID = original.Terminal
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, request.Snapshot.Ref.Target, original, state)
	return receipt, err
}
