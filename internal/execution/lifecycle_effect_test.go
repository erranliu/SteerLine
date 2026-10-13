package execution

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

type creationCheckpointPort struct {
	b.LifecyclePort
	executes, reconciles int
	checkpoint           bool
}

func (p *creationCheckpointPort) Execute(_ context.Context, r b.LifecycleRequest) (b.LifecycleReceipt, b.LifecycleObserver, error) {
	p.executes++
	receipt := b.LifecycleReceipt{Operation: r.Operation, Disposition: b.OperationUnknown}
	if p.checkpoint {
		receipt.Operation.Receipt = []byte(`{"original_workspace":"instance-1"}`)
	}
	return receipt, nil, errors.New("terminal query unavailable")
}

func (p *creationCheckpointPort) Reconcile(_ context.Context, r b.LifecycleRequest) (b.LifecycleReceipt, b.LifecycleObserver, error) {
	p.reconciles++
	var saved b.LifecycleReceipt
	if json.Unmarshal(r.Operation.Receipt, &saved) != nil || saved.ValidateFor(r) != nil || string(saved.Operation.Receipt) != `{"original_workspace":"instance-1"}` {
		return b.LifecycleReceipt{Operation: r.Operation, Disposition: b.OperationUnknown}, nil, errors.New("missing creation checkpoint")
	}
	return b.LifecycleReceipt{Operation: r.Operation, Disposition: b.OperationConfirmed}, nil, nil
}

func TestLifecycleEffectForwardsWorkspaceCreationCheckpoint(t *testing.T) {
	for _, checkpoint := range []bool{true, false} {
		port := &creationCheckpointPort{checkpoint: checkpoint}
		ref := b.ExecutionRef{RunID: strings.Repeat("a", 32), AttemptID: strings.Repeat("b", 32)}
		request := b.LifecycleRequest{
			Snapshot:  b.LifecycleSnapshot{Session: b.SessionContext{ProjectID: "project", ExecutionID: "execution", WorkflowRunID: ref.RunID, AttemptID: ref.AttemptID}, ConfigurationDigest: "config", ResourceGeneration: "generation"},
			Input:     b.LifecycleInput{Workspace: &b.WorkspaceIntent{BaseCommit: strings.Repeat("c", 40)}},
			Operation: b.Operation{ID: "original-workspace", Action: string(b.LifecyclePrepareWorkspace), PayloadDigest: "digest", Ref: &ref, LifecycleIdentity: &b.LifecycleOperationIdentity{ProjectID: "project", ExecutionID: "execution", ConfigurationDigest: "config", ResourceGeneration: "generation", InputBindingDigest: "binding", Ref: ref}},
		}
		if err := request.Validate(); err != nil {
			t.Fatal(err)
		}
		result := invokeLifecycleEffect(context.Background(), port, request, false)
		if (result.err == nil) != checkpoint || (result.receipt.Disposition == b.OperationConfirmed) != checkpoint || port.executes != 1 || port.reconciles != 1 {
			t.Fatalf("checkpoint or non-replay contract lost: %+v, %+v", port, result)
		}
	}
}
