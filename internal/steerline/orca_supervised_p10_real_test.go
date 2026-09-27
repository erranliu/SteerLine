package steerline

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// TestSupervisedOrcaRealP10ReplyGate exercises the actual supervised reply
// entry point against one still-open question on the isolated home target.
// It is intentionally opt-in and never creates a worker or task itself.
func TestSupervisedOrcaRealP10ReplyGate(t *testing.T) {
	if os.Getenv("STEERLINE_REAL_P10_REPLY_PROBE") != "1" {
		t.Skip("set STEERLINE_REAL_P10_REPLY_PROBE=1 with an exact live question")
	}
	need := func(name string) string {
		t.Helper()
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			t.Fatalf("%s is required", name)
		}
		return value
	}
	c := Config{
		Host: "orca_supervised", CommandTimeout: "30s",
		Orca: OrcaConfig{Executable: "orca", Supervised: &SupervisedOrcaConfig{
			ConnectionID:        need("STEERLINE_REAL_P10_CONNECTION_ID"),
			EnvironmentID:       need("STEERLINE_REAL_P10_ENVIRONMENT_ID"),
			CoordinatorTerminal: need("STEERLINE_REAL_P10_COORDINATOR_TERMINAL"),
		}},
	}
	started := supervisedStartReceipt{
		RunID:           need("STEERLINE_REAL_P10_RUN_ID"),
		TaskID:          need("STEERLINE_REAL_P10_TASK_ID"),
		DispatchID:      need("STEERLINE_REAL_P10_DISPATCH_ID"),
		WorktreeID:      need("STEERLINE_REAL_P10_WORKTREE_ID"),
		CenterRuntimeID: need("STEERLINE_REAL_P10_CENTER_RUNTIME_ID"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	b := orcaSupervisedBackend{config: c}
	observed, err := b.showWorker(ctx, started)
	if err != nil || observed.Liveness != "live" {
		t.Fatalf("exact home worker is not live: observation=%+v error=%v", observed, err)
	}
	target := targetFor(c)
	target.OrchestrationRun = started.RunID
	target.ExternalTask = started.TaskID
	target.Dispatch = started.DispatchID
	target.ExecutionHost = observed.Identity.ExecutionHost
	target.WorktreeID = started.WorktreeID
	ref := ExecutionRef{Version: ExecutionRecordVersion, Backend: BackendOrca, Protocol: ProtocolOrcaSupervised, Target: target, External: observed.Identity}
	questionID := need("STEERLINE_REAL_P10_QUESTION_ID")
	oversized := "x" + strings.Repeat("\n", 4200)
	serialized, err := json.Marshal(struct {
		QuestionID string `json:"question_id"`
		Body       string `json:"body"`
		Run        string `json:"run"`
		From       string `json:"from"`
		Dispatch   string `json:"dispatch"`
	}{questionID, oversized, started.RunID, c.Orca.Supervised.CoordinatorTerminal, started.DispatchID})
	if err != nil || len(serialized) <= maxSupervisedMessageBytes {
		t.Fatalf("probe answer did not exceed the serialized limit: bytes=%d error=%v", len(serialized), err)
	}
	if _, err = b.replyQuestion(ctx, ref, questionID, oversized); !isBackendError(err, BackendRejected) || !strings.Contains(err.Error(), "serialized message exceeds") {
		t.Fatalf("oversized answer did not fail before Orca send: %v", err)
	}
	t.Logf("oversized serialized reply rejected before observation/send: %d bytes, question=%s, dispatch=%s", len(serialized), questionID, started.DispatchID)
	if os.Getenv("STEERLINE_REAL_P10_SEND_BOUNDED_REPLY") == "1" {
		requestID, replyErr := b.replyQuestion(ctx, ref, questionID, need("STEERLINE_REAL_P10_BOUNDED_REPLY"))
		if replyErr != nil || requestID == "" {
			t.Fatalf("bounded reply to exact home question was not accepted: request=%q error=%v", requestID, replyErr)
		}
		t.Logf("bounded reply accepted: request=%s question=%s dispatch=%s", requestID, questionID, started.DispatchID)
	}
}
