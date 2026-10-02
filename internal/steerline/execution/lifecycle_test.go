package execution

import (
	"testing"
)

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
