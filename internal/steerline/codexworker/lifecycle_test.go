package codexworker

import (
	"context"
	"strings"
	"testing"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func TestStartTurnFailureRetainsOriginalSessionIdentity(t *testing.T) {
	lifecycle := &ExecutionLifecycle{}
	process := executionapi.ProcessIdentity{Kind: "os", PID: 123, Created: "original"}
	binding := Binding{Process: &process, Endpoint: "ws://127.0.0.1:1", Generation: "session-generation", SessionID: "00000000-0000-0000-0000-000000000008"}
	snapshot := executionapi.LifecycleSnapshot{
		Ref:                 executionapi.ExecutionRef{External: process},
		Session:             executionapi.SessionContext{ProjectID: strings.Repeat("a", 32), ExecutionID: strings.Repeat("b", 32), AttemptID: strings.Repeat("c", 32)},
		ConfigurationDigest: "fixed-config", ResourceGeneration: strings.Repeat("d", 32),
	}
	snapshot.Implementation = lifecycle.bindingReceipt(snapshot, binding).Implementation
	input := executionapi.TurnInput{CommandID: strings.Repeat("e", 32), Prompt: "original prompt"}
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "resume_failure", true: "changed_prepared_binding"}[changed], func(t *testing.T) {
			if changed {
				original := binding
				original.Generation = "other-generation"
				lifecycle.prepared = &SessionHandle{Binding: original, input: input}
			}
			receipt, observer, err := lifecycle.startTurn(context.Background(), snapshot, input)
			if err == nil || observer != nil || receipt.Disposition != executionapi.OperationUnknown ||
				receipt.State.SessionID != binding.SessionID || receipt.State.SessionGeneration != binding.Generation ||
				receipt.Process == nil || *receipt.Process != process || receipt.State.TurnStarted || lifecycle.prepared != nil {
				t.Fatalf("failed turn lost original identity or advanced input: %+v / %v", receipt, err)
			}
		})
	}
}
