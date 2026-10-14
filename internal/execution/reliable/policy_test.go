package reliable

import (
	"context"
	"errors"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
	"reflect"
	"testing"
	"time"
)

type preparationFixture struct {
	caps  b.PreparationCapabilities
	facts b.PreparationFacts
	read  int
}

func (p *preparationFixture) PolicyCapabilities() b.LifecyclePolicyCapabilities {
	return b.LifecyclePolicyCapabilities{Preparation: p.caps}
}
func (p *preparationFixture) PreparationFacts(context.Context, b.LifecycleSnapshot) (b.PreparationFacts, error) {
	p.read++
	return p.facts, nil
}
func preparationSnapshot() b.LifecycleSnapshot {
	return b.LifecycleSnapshot{WorkspacePlan: b.WorkspaceIntent{Path: "original", Branch: "fixed", BaseCommit: "base"}, PendingTurn: &b.TurnInput{CommandID: "command", Prompt: "saved"}}
}

func TestReliablePreparationFacts(t *testing.T) {
	p := &preparationFixture{caps: b.PreparationCapabilities{Workspace: b.MechanismApplicable, Resource: b.MechanismApplicable, Session: b.MechanismApplicable}}
	s := preparationSnapshot()
	for _, tc := range []struct {
		facts  b.PreparationFacts
		action b.LifecycleAction
	}{
		{b.PreparationFacts{Workspace: b.GateRequired}, b.LifecyclePrepareWorkspace},
		{b.PreparationFacts{Workspace: b.GateReady, Resource: b.GateRequired, ResourceRequired: &b.ResourceIntent{Purpose: "native-launch", Objective: "saved-command"}}, b.LifecycleCreateResource},
		{b.PreparationFacts{Workspace: b.GateReady, Resource: b.GateReady, Session: b.GateRequired}, b.LifecyclePrepareSession},
		{b.PreparationFacts{Workspace: b.GateReady, Resource: b.GateReady, Session: b.GateReady}, b.LifecycleStartTurn},
	} {
		p.facts = tc.facts
		next, err := Next(context.Background(), p, s)
		if err != nil || next.Action != tc.action {
			t.Fatalf("facts=%+v next=%+v err=%v", tc.facts, next, err)
		}
		if next.Input.Turn != nil && !reflect.DeepEqual(*next.Input.Turn, *s.PendingTurn) {
			t.Fatal("changed saved input")
		}
	}
}

func TestReliablePreparationApplicability(t *testing.T) {
	s := preparationSnapshot()
	p := &preparationFixture{caps: b.PreparationCapabilities{Workspace: b.MechanismApplicable, Resource: b.MechanismNotApplicable, Session: b.MechanismApplicable}, facts: b.PreparationFacts{Workspace: b.GateReady, Resource: b.GateNotApplicable, Session: b.GateRequired}}
	next, err := Next(context.Background(), p, s)
	if err != nil || next.Action != b.LifecyclePrepareSession {
		t.Fatalf("not applicable resource was not skipped: %+v %v", next, err)
	}
	for _, tc := range []struct {
		name string
		decl b.Applicability
		fact b.PreparationGate
		want b.PreparationGate
	}{
		{"required unsupported", b.MechanismUnsupported, b.GateNotApplicable, b.GateUnsupported},
		{"unknown native evidence", b.MechanismApplicable, b.GateUnverified, b.GateUnverified},
		{"zero native evidence", b.MechanismApplicable, "", b.GateUnverified},
		{"zero declaration", "", b.GateReady, b.GateUnverified},
		{"false opt out", b.MechanismApplicable, b.GateNotApplicable, b.GateUnverified},
		{"conflicting opt out", b.MechanismNotApplicable, b.GateRequired, b.GateUnverified},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p.caps.Session, p.facts.Session = tc.decl, tc.fact
			next, err := Next(context.Background(), p, s)
			var gap *GateError
			if !errors.As(err, &gap) || gap.Status != tc.want || next.Action != "" || next.Done {
				t.Fatalf("gate bypassed: %+v %v", next, err)
			}
		})
	}
}

func TestReliableInputOutcomeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		turn, input, session string
		unsent               bool
	}{
		{"", "", "", true}, {"rejected", "", "confirmed", true}, {"accepted", "", "confirmed", false}, {"outcome_unknown", "", "", false}, {"", "in_flight", "", false}, {"", "", "outcome_unknown", false},
	} {
		if got := InputNotSubmitted(tc.turn, tc.input, tc.session); got != tc.unsent {
			t.Fatalf("%+v got=%v", tc, got)
		}
	}
	receipt := b.LifecycleReceipt{Disposition: b.OperationAccepted, State: b.LifecycleStateFacts{InputAccepted: true}}
	if StartAccepted(receipt, nil, true) {
		t.Fatal("acceptance inferred start")
	}
	status, _ := Outcome(receipt, errors.New("receipt lost"))
	if status != "outcome_unknown" {
		t.Fatal(status)
	}
}

func TestReliableRecoveryAndAuthorization(t *testing.T) {
	e := b.LifecycleRecoveryEvidence{SessionAvailable: true, SessionIdle: true, SessionRebindable: true, ProcessAbsent: true, WritersAbsent: true}
	if !CanContinueOriginal(e) || ValidateRestoration(e, "original", false) != nil {
		t.Fatal("same session restoration rejected")
	}
	if ValidateRestoration(e, "original", true) == nil {
		t.Fatal("unknown effects bypassed recovery")
	}
	e.WritersAbsent = false
	if CanContinueOriginal(e) || ValidateRestoration(e, "original", false) == nil {
		t.Fatal("process death inferred writer absence")
	}
	available := true
	prior := RecoveryRevision{State: "running", SessionAvailable: &available, Version: 4}
	unchanged := RecoveryChange(prior, RecoveryFacts{WorkerLive: true}, true, false)
	if unchanged.Version != 4 {
		t.Fatal("same facts invalidated consent")
	}
	changed := RecoveryChange(prior, RecoveryFacts{WorkerLive: true}, true, true)
	if changed.Version != 5 || changed.State != "unverified" {
		t.Fatal("unknown effects did not invalidate consent")
	}
	if !(ContinuationConstraints{PriorOwner: true}).RequiresAuthorization() {
		t.Fatal("prior service bypassed authority")
	}
	at := time.Now()
	if PreparationWindow(at, at, time.Second) != 0 {
		t.Fatal("expired preparation renewed its budget")
	}
}

func TestReliableLateFacts(t *testing.T) {
	original := ObservationIdentity{ExecutionID: "one", AttemptID: "old", Generation: "g", RecoveryVersion: 2}
	current := original
	current.AttemptID = "new"
	if CurrentObservation(current, original) {
		t.Fatal("late attempt accepted")
	}
	current = original
	current.ExecutionID = "sibling"
	if CurrentObservation(current, original) {
		t.Fatal("sibling observation accepted")
	}
	s := b.SessionContext{CurrentCommandID: "command", CurrentTurnID: "turn", SessionID: "session"}
	e := b.TurnEvidence{CommandID: "command", TurnID: "turn", SessionID: "session", Started: true, Completed: true, Succeeded: true, LastMessage: "final"}
	result, err := NormalizeResult(s, e)
	if err != nil || result.ContentVerified {
		t.Fatal("missing provenance became verified")
	}
	e.SessionID = "replacement"
	if _, err := NormalizeResult(s, e); !errors.Is(err, ErrStale) {
		t.Fatal("replacement accepted")
	}
	value := BindingSnapshot{Target: b.ExecutionTarget{ID: "original"}, SessionID: "session"}
	receipt := b.LifecycleReceipt{Target: &b.ExecutionTarget{ID: "changed"}, State: b.LifecycleStateFacts{SessionID: "replacement"}}
	got, err := MergeReceipt(value, receipt)
	if err == nil || !reflect.DeepEqual(got, value) {
		t.Fatal("invalid partial receipt leaked a mutation")
	}
}
