package execution

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
	m "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

type creationCheckpointPort struct {
	b.LifecyclePort
	executes, reconciles int
	checkpoint           bool
	unavailable          bool
}

func (p *creationCheckpointPort) PolicyCapabilities() b.LifecyclePolicyCapabilities {
	return b.LifecyclePolicyCapabilities{Preparation: b.PreparationCapabilities{Workspace: b.MechanismApplicable, Resource: b.MechanismNotApplicable, Session: b.MechanismApplicable}}
}
func (p *creationCheckpointPort) PreparationFacts(context.Context, b.LifecycleSnapshot) (b.PreparationFacts, error) {
	return b.PreparationFacts{Workspace: b.GateRequired, Resource: b.GateNotApplicable, Session: b.GateUnverified}, nil
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
	if p.unavailable {
		return b.LifecycleReceipt{Operation: r.Operation, Disposition: b.OperationUnknown}, nil, errors.New("reconciliation unavailable")
	}
	var saved b.LifecycleReceipt
	if json.Unmarshal(r.Operation.Receipt, &saved) != nil || saved.ValidateFor(r) != nil || string(saved.Operation.Receipt) != `{"original_workspace":"instance-1"}` {
		return b.LifecycleReceipt{Operation: r.Operation, Disposition: b.OperationUnknown}, nil, errors.New("missing creation checkpoint")
	}
	return b.LifecycleReceipt{Operation: r.Operation, Disposition: b.OperationConfirmed}, nil, nil
}

func TestLifecycleEffectForwardsWorkspaceCreationCheckpoint(t *testing.T) {
	for _, scenario := range []struct{ checkpoint, unavailable bool }{{true, false}, {false, false}, {true, true}} {
		checkpoint := scenario.checkpoint
		port := &creationCheckpointPort{checkpoint: checkpoint, unavailable: scenario.unavailable}
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
		if port.reconciles != 0 {
			t.Fatal("background mutation authorized follow-up reconciliation")
		}
		run := &reconstruction.RunView{RunID: ref.RunID}
		service := &Service{domain: reconstruction.Snapshot{WorkflowRuns: []m.WorkflowRunRecord{{WorkflowRunID: ref.RunID, Status: "queued"}}}}
		if err := service.consumeLifecycleEffect(context.Background(), run, port, result, func(_ *reconstruction.RunView, next lifecycleEffectResult) error { result = next; return nil }, false); err != nil {
			t.Fatal(err)
		}
		confirmed := checkpoint && !scenario.unavailable
		if (result.err == nil) != confirmed || (result.receipt.Disposition == b.OperationConfirmed) != confirmed || port.executes != 1 || port.reconciles != 1 {
			t.Fatalf("checkpoint or non-replay contract lost: %+v, %+v", port, result)
		}
		if scenario.unavailable && (!strings.Contains(result.err.Error(), "terminal query unavailable") || !strings.Contains(result.err.Error(), "reconciliation unavailable") || string(result.receipt.Operation.Receipt) != `{"original_workspace":"instance-1"}`) {
			t.Fatal("failed reconciliation lost the original creation evidence or diagnostic", result)
		}
	}
}
