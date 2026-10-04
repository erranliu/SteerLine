package backend

import (
	"path/filepath"
	"testing"
)

func TestHostMutationRequiresOneFixedOriginalTarget(t *testing.T) {
	probe := ProbeIdentity{Execution: "execution", Turn: "turn", Nonce: "nonce", ProfileDigest: "profile", Runtime: "runtime", Terminal: "terminal", Incarnation: "incarnation", Worktree: filepath.Join(t.TempDir(), "worktree")}
	if err := (HostActionInput{Kind: HostPrepareProbeFiles, Probe: &probe}).Validate(); err != nil {
		t.Fatalf("valid probe mutation rejected: %v", err)
	}
	if err := (HostActionInput{Kind: HostPrepareProbeFiles, Probe: &probe, Process: &ProcessIdentity{Kind: "orca"}}).Validate(); err == nil {
		t.Fatal("ambiguous host mutation union was accepted")
	}
	authorization := &HostActionAuthorization{ExecutionID: "execution", AttemptID: "attempt", TurnID: "turn", CommandID: "command", BlockerID: "blocker", BlockerVersion: 1}
	if err := (HostActionInput{Kind: HostOpenOriginal, Process: &ProcessIdentity{Kind: "orca", Terminal: "terminal", RuntimeID: "runtime", Incarnation: "incarnation"}, Authorization: authorization}).Validate(); err != nil {
		t.Fatalf("fixed original host target rejected: %v", err)
	}
	if err := (LifecycleCandidate{Action: LifecycleHostAction, Input: LifecycleInput{Host: &HostActionInput{Kind: HostOpenOriginal}}}).Validate(); err == nil {
		t.Fatal("host mutation without an original process identity was accepted")
	}
}

func TestLifecycleCandidateRequiresOneMatchingInput(t *testing.T) {
	tests := []struct {
		name      string
		candidate LifecycleCandidate
		valid     bool
	}{
		{
			name: "turn preparation",
			candidate: LifecycleCandidate{
				Action: LifecyclePrepareSession,
				Input:  LifecycleInput{Turn: &TurnInput{CommandID: "command", Prompt: "prompt"}},
			},
			valid: true,
		},
		{
			name: "mismatched action",
			candidate: LifecycleCandidate{
				Action: LifecycleAcknowledge,
				Input:  LifecycleInput{Turn: &TurnInput{CommandID: "command", Prompt: "prompt"}},
			},
		},
		{
			name: "ambiguous union",
			candidate: LifecycleCandidate{
				Action: LifecycleStartTurn,
				Input:  LifecycleInput{Turn: &TurnInput{CommandID: "command", Prompt: "prompt"}, Ack: &AcknowledgeInput{DeliveryID: "delivery"}},
			},
		},
		{
			name:      "unknown action",
			candidate: LifecycleCandidate{Action: "native_dispatch", Input: LifecycleInput{Turn: &TurnInput{CommandID: "command", Prompt: "prompt"}}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.candidate.Validate()
			if (err == nil) != test.valid {
				t.Fatalf("candidate validity = %v, error = %v", err == nil, err)
			}
		})
	}
}

func TestLifecycleRequestRequiresOriginalDurableOperation(t *testing.T) {
	request := LifecycleRequest{
		Operation: Operation{ID: "operation", Action: string(LifecycleStartTurn), PayloadDigest: "digest", Ref: &ExecutionRef{}, LifecycleIdentity: &LifecycleOperationIdentity{InputBindingDigest: "binding-digest"}},
		Input:     LifecycleInput{Turn: &TurnInput{CommandID: "command", Prompt: "prompt"}},
	}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	request.Operation.ID = ""
	if err := request.Validate(); err == nil {
		t.Fatal("request without a durable operation identity was accepted")
	}
}

func TestLifecycleReceiptCloneOwnsImplementationPayloadAndFacts(t *testing.T) {
	receipt := LifecycleReceipt{
		Implementation: []ImplementationDataEnvelope{{Payload: []byte(`{"state":1}`)}},
		Facts:          []EventFact{{PRCandidates: []string{"https://example.test/pull/1"}}},
	}
	cloned := receipt.Clone()
	cloned.Implementation[0].Payload[0] = '['
	cloned.Facts[0].PRCandidates[0] = "changed"
	if string(receipt.Implementation[0].Payload) != `{"state":1}` || receipt.Facts[0].PRCandidates[0] != "https://example.test/pull/1" {
		t.Fatal("lifecycle receipt clone aliases implementation data or event facts")
	}
}
