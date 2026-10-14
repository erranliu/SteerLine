package reliable

import (
	"context"
	"errors"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
	"testing"
)

type effectFixture struct {
	b.LifecyclePort
	preparationFixture
	executes, reconciles int
}

func (p *effectFixture) PolicyCapabilities() b.LifecyclePolicyCapabilities {
	return p.preparationFixture.PolicyCapabilities()
}
func (p *effectFixture) PreparationFacts(ctx context.Context, s b.LifecycleSnapshot) (b.PreparationFacts, error) {
	return p.preparationFixture.PreparationFacts(ctx, s)
}
func (p *effectFixture) Execute(_ context.Context, r b.LifecycleRequest) (b.LifecycleReceipt, b.LifecycleObserver, error) {
	p.executes++
	return b.LifecycleReceipt{Operation: r.Operation, Disposition: b.OperationUnknown, State: b.LifecycleStateFacts{SessionID: "partial-original"}}, nil, errors.New("receipt lost")
}
func (p *effectFixture) Reconcile(_ context.Context, r b.LifecycleRequest) (b.LifecycleReceipt, b.LifecycleObserver, error) {
	p.reconciles++
	return b.LifecycleReceipt{Operation: r.Operation, Disposition: b.OperationUnknown}, nil, errors.New("original request unavailable")
}
func originalRequest() b.LifecycleRequest {
	s := preparationSnapshot()
	s.Session = b.SessionContext{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", WorkflowRunID: "association"}
	s.Ref = b.ExecutionRef{RunID: "association", AttemptID: "attempt"}
	s.ConfigurationDigest, s.ResourceGeneration = "config", "generation"
	op := b.Operation{ID: "original", PayloadDigest: "digest", Action: string(b.LifecycleStartTurn), Status: b.OperationInFlight, Ref: &s.Ref, LifecycleIdentity: &b.LifecycleOperationIdentity{ProjectID: "project", ExecutionID: "execution", ConfigurationDigest: "config", ResourceGeneration: "generation", InputBindingDigest: b.LifecycleBindingDigest(s), Ref: s.Ref}}
	return b.LifecycleRequest{Snapshot: s, Operation: op, Input: b.LifecycleInput{Turn: s.PendingTurn}}
}
func TestReliableUnknownEffectAndGateCannotResubmit(t *testing.T) {
	p := &effectFixture{preparationFixture: preparationFixture{caps: b.PreparationCapabilities{Workspace: b.MechanismApplicable, Resource: b.MechanismNotApplicable, Session: b.MechanismApplicable}, facts: b.PreparationFacts{Workspace: b.GateReady, Resource: b.GateNotApplicable, Session: b.GateUnverified}}}
	request := originalRequest()
	result := Invoke(context.Background(), p, p, request, ExecuteNewIntent)
	var gap *GateError
	if !errors.As(result.Err, &gap) || p.executes != 0 {
		t.Fatal("unknown readiness reached input mutation")
	}
	p.caps.Session = b.MechanismUnsupported
	p.facts.Session = b.GateNotApplicable
	result = Invoke(context.Background(), p, p, request, ExecuteNewIntent)
	if !errors.As(result.Err, &gap) || gap.Status != b.GateUnsupported || p.executes != 0 {
		t.Fatal("unsupported required capability was skipped")
	}
	p.caps.Session = b.MechanismApplicable
	p.facts.Session = b.GateReady
	first := Invoke(context.Background(), p, p, request, ExecuteNewIntent)
	if !first.Uncertain || p.executes != 1 {
		t.Fatal("lost mutation receipt was not unknown")
	}
	for i := 0; i < 2; i++ {
		result = Invoke(context.Background(), p, p, request, ReconcileOriginalIntent)
		result = ReconciledResult(result, &first)
		if result.Receipt.State.SessionID != "partial-original" {
			t.Fatal("partial original binding discarded")
		}
	}
	request.Operation.Status = b.OperationUnknown
	_ = Invoke(context.Background(), p, p, request, ExecuteNewIntent)
	if p.executes != 1 || p.reconciles != 3 {
		t.Fatal("caller bypassed original unknown-operation reconciliation")
	}
	request.Operation.Status = b.OperationPending
	_ = Invoke(context.Background(), p, p, request, ExecuteNewIntent)
	if p.executes != 1 {
		t.Fatal("uncommitted pending intent reached the adapter")
	}
	if p.executes != 1 || p.reconciles != 3 {
		t.Fatal("unknown request was resubmitted")
	}
}
func TestReliableTurnIdentityAndNotifications(t *testing.T) {
	state := TurnState{}
	next, err := BeginTurn(state, "session", "first", TurnAuthority{Allowed: true}, state.UpdatedAt)
	if err != nil || next.Number != 1 {
		t.Fatal(err)
	}
	if _, err := BeginTurn(next, "replacement", "other", TurnAuthority{Allowed: true}, state.UpdatedAt); err == nil {
		t.Fatal("session replaced")
	}
	stopped, err := StopTurn(next, "first", true, state.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BeginTurn(stopped, "session", "second", TurnAuthority{Allowed: true}, state.UpdatedAt)
	if err != nil || second.Number != 2 {
		t.Fatal(err)
	}
	if err := CurrentTurn(second, "first"); !errors.Is(err, ErrStale) {
		t.Fatal("late question changed current turn")
	}
	if _, err := StopTurn(second, "first", true, state.UpdatedAt); !errors.Is(err, ErrStale) {
		t.Fatal("late stop changed current turn")
	}
	duplicate, err := BeginTurn(second, "session", "second", TurnAuthority{}, state.UpdatedAt)
	if err != nil || duplicate != second {
		t.Fatal("duplicate start changed state")
	}
}

func TestReliableFreshActionKeepsOriginalIntent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*b.LifecycleRequest, *effectFixture)
	}{
		{"changed workspace", func(r *b.LifecycleRequest, p *effectFixture) {
			r.Operation.Action = string(b.LifecyclePrepareWorkspace)
			intent := r.Snapshot.WorkspacePlan
			intent.Path = "replacement"
			r.Input = b.LifecycleInput{Workspace: &intent}
			p.facts.Workspace = b.GateRequired
		}},
		{"changed command", func(r *b.LifecycleRequest, _ *effectFixture) { r.Input.Turn.CommandID = "replacement" }},
		{"changed prompt", func(r *b.LifecycleRequest, _ *effectFixture) { r.Input.Turn.Prompt = "replacement" }},
		{"changed write grant", func(r *b.LifecycleRequest, _ *effectFixture) {
			r.Input.Turn.LinkedWorktreeArgs = []string{"replacement"}
		}},
		{"missing pending input", func(r *b.LifecycleRequest, _ *effectFixture) { r.Snapshot.PendingTurn = nil }},
		{"session already ready", func(r *b.LifecycleRequest, _ *effectFixture) { r.Operation.Action = string(b.LifecyclePrepareSession) }},
		{"session not applicable", func(r *b.LifecycleRequest, p *effectFixture) {
			r.Operation.Action = string(b.LifecyclePrepareSession)
			p.caps.Session, p.facts.Session = b.MechanismNotApplicable, b.GateNotApplicable
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &effectFixture{preparationFixture: preparationFixture{caps: b.PreparationCapabilities{Workspace: b.MechanismApplicable, Resource: b.MechanismNotApplicable, Session: b.MechanismApplicable}, facts: b.PreparationFacts{Workspace: b.GateReady, Resource: b.GateNotApplicable, Session: b.GateReady}}}
			request := originalRequest().Clone()
			tc.change(&request, p)
			result := Invoke(context.Background(), p, p, request, ExecuteNewIntent)
			if result.Err == nil || p.executes != 0 || p.reconciles != 0 || result.Uncertain || result.Receipt.Disposition != b.OperationRejected {
				t.Fatalf("changed/unnecessary intent reached adapter: %+v executes=%d reconciles=%d", result, p.executes, p.reconciles)
			}
		})
	}
}

type unverifiedReceiptFixture struct {
	effectFixture
	wrongIdentity bool
}

func (p *unverifiedReceiptFixture) Execute(_ context.Context, r b.LifecycleRequest) (b.LifecycleReceipt, b.LifecycleObserver, error) {
	p.executes++
	receipt := b.LifecycleReceipt{Operation: r.Operation}
	if p.wrongIdentity {
		receipt.Operation.ID = "foreign"
		receipt.Disposition = b.OperationConfirmed
	}
	return receipt, nil, nil
}

func TestReliableUnverifiedReceiptRequiresOriginalReconciliation(t *testing.T) {
	for _, wrongIdentity := range []bool{false, true} {
		p := &unverifiedReceiptFixture{effectFixture: effectFixture{preparationFixture: preparationFixture{caps: b.PreparationCapabilities{Workspace: b.MechanismApplicable, Resource: b.MechanismNotApplicable, Session: b.MechanismApplicable}, facts: b.PreparationFacts{Workspace: b.GateReady, Resource: b.GateNotApplicable, Session: b.GateReady}}}, wrongIdentity: wrongIdentity}
		request := originalRequest()
		result := Invoke(context.Background(), p, p, request, ExecuteNewIntent)
		if result.Err == nil || !result.Uncertain || p.executes != 1 {
			t.Fatalf("unverified receipt was not an uncertain effect: %+v", result)
		}
		reconcile, needed := ReconciliationRequest(result)
		if !needed || reconcile.Operation.ID != request.Operation.ID {
			t.Fatal("receipt loss did not retain the original reconciliation identity")
		}
		_ = Invoke(context.Background(), p, p, reconcile, ReconcileOriginalIntent)
		if p.executes != 1 || p.reconciles != 1 {
			t.Fatal("unverified receipt was resubmitted")
		}
	}
}

func TestReliableReconciliationRejectsForeignIntent(t *testing.T) {
	p := &effectFixture{}
	request := originalRequest()
	request.Operation.LifecycleIdentity.ExecutionID = "foreign"
	result := Invoke(context.Background(), p, p, request, ReconcileOriginalIntent)
	if result.Err == nil || p.executes != 0 || p.reconciles != 0 {
		t.Fatal("foreign saved intent reached original reconciliation")
	}
}

func TestReliableOutcomeCannotTreatMissingReceiptAsSuccess(t *testing.T) {
	for _, disposition := range []b.OperationStatus{"", b.OperationPending, b.OperationInFlight, "invalid"} {
		status, diagnostic := Outcome(b.LifecycleReceipt{Disposition: disposition}, nil)
		if status != string(b.OperationUnknown) || diagnostic == "" {
			t.Fatalf("unverified disposition %q returned status=%q diagnostic=%q", disposition, status, diagnostic)
		}
	}
}

type expiringQualificationFixture struct {
	effectFixture
	cancel context.CancelFunc
}

func (p *expiringQualificationFixture) PreparationFacts(context.Context, b.LifecycleSnapshot) (b.PreparationFacts, error) {
	p.cancel()
	return p.facts, nil
}

func TestReliableExpiredIntentCannotExecute(t *testing.T) {
	for _, beforeQualification := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		p := &expiringQualificationFixture{effectFixture: effectFixture{preparationFixture: preparationFixture{caps: b.PreparationCapabilities{Workspace: b.MechanismApplicable, Resource: b.MechanismNotApplicable, Session: b.MechanismApplicable}, facts: b.PreparationFacts{Workspace: b.GateReady, Resource: b.GateNotApplicable, Session: b.GateReady}}}, cancel: cancel}
		if beforeQualification {
			cancel()
		}
		result := Invoke(ctx, p, p, originalRequest(), ExecuteNewIntent)
		if !errors.Is(result.Err, context.Canceled) || p.executes != 0 || result.Uncertain || result.Receipt.Disposition != b.OperationRejected {
			t.Fatal("expired intent entered a mutation instead of a definite unsent rejection")
		}
	}
}
