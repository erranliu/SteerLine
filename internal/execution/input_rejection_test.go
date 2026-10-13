package execution

import (
	"errors"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
	"testing"
)

func TestWorkerInputRejectedReceipt(t *testing.T) {
	s, run := neutralAuditService(t)
	fixed, err := s.workerFlowContext(run)
	require(t, err)
	input := b.TurnInput{CommandID: newID(), Prompt: "original input", Purpose: "business"}
	fixed.execution.PendingCommandID = input.CommandID
	require(t, s.commitWorker(run))
	key := "worker-turn:" + fixed.execution.ExecutionID + ":" + input.CommandID
	_, err = s.startWorkerOperation(run, "start_worker_turn", key, input)
	require(t, err)
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, &input)
	require(t, err)
	req := lifecycleRequest(snapshot, *s.operationByKey(key), b.LifecycleStartTurn, b.LifecycleInput{Turn: &input})
	receipt := b.LifecycleReceipt{Operation: req.Operation, Disposition: b.OperationRejected}
	require(t, s.applyAppServerTurnReceipt(run, req, receipt, nil, errors.New("explicitly not submitted")))
	if s.operationByKey(key).Status != "rejected" {
		t.Fatal("known rejection changed to unknown", s.operationByKey(key).Status)
	}
}
