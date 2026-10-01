package steerline

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
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
	c := Config{Host: "orca_supervised", Agent: "codex", CommandTimeout: "30s"}
	setTestImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: "codex", Model: "gpt-6-luna", ReasoningEffort: "high"}, &orcaworker.ProfileConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "5m"}, &remote.ProfileConfig{
		ConnectionID:            need("STEERLINE_REAL_P10_CONNECTION_ID"),
		EnvironmentID:           need("STEERLINE_REAL_P10_ENVIRONMENT_ID"),
		RepositorySelector:      need("STEERLINE_REAL_P10_REPOSITORY_SELECTOR"),
		CoordinatorTerminal:     need("STEERLINE_REAL_P10_COORDINATOR_TERMINAL"),
		PollInterval:            "2s",
		CapabilityReportVersion: 1,
		RemoteCancelAvailable:   false,
	})
	started := supervisedStartReceipt{
		RunID:           need("STEERLINE_REAL_P10_RUN_ID"),
		TaskID:          need("STEERLINE_REAL_P10_TASK_ID"),
		DispatchID:      need("STEERLINE_REAL_P10_DISPATCH_ID"),
		WorktreeID:      need("STEERLINE_REAL_P10_WORKTREE_ID"),
		CenterRuntimeID: need("STEERLINE_REAL_P10_CENTER_RUNTIME_ID"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	observed, err := remoteWorkerClient(c).Show(ctx, started)
	if err != nil || observed.Liveness != "live" {
		t.Fatalf("exact home worker is not live: observation=%+v error=%v", observed, err)
	}
	target := targetFor(c)
	target.OrchestrationRun = started.RunID
	target.ExternalTask = started.TaskID
	target.Dispatch = started.DispatchID
	target.ExecutionHost = observed.Identity.ExecutionHost
	target.WorktreeID = started.WorktreeID
	ref := executionapi.ExecutionRef{RunID: need("STEERLINE_REAL_P10_WORKFLOW_RUN_ID"), Version: executionapi.ExecutionRecordVersion, Backend: executionapi.BackendOrca, Protocol: executionapi.ProtocolOrcaSupervised, Target: target, External: observed.Identity}
	questionID := need("STEERLINE_REAL_P10_QUESTION_ID")
	oversized := "x" + strings.Repeat("\n", 4200)
	serialized, err := json.Marshal(struct {
		QuestionID string `json:"question_id"`
		Body       string `json:"body"`
		Run        string `json:"run"`
		From       string `json:"from"`
		Dispatch   string `json:"dispatch"`
	}{questionID, oversized, started.RunID, remoteProfileSettings(c).CoordinatorTerminal, started.DispatchID})
	if err != nil || len(serialized) <= maxSupervisedMessageBytes {
		t.Fatalf("probe answer did not exceed the serialized limit: bytes=%d error=%v", len(serialized), err)
	}
	if _, err = remoteWorkerClient(c).Reply(ctx, remoteWorkerRef(ref), questionID, oversized); !isBackendError(remoteWorkerError(err), executionapi.BackendRejected) || !strings.Contains(err.Error(), "serialized message exceeds") {
		t.Fatalf("oversized answer did not fail before Orca send: %v", err)
	}
	t.Logf("oversized serialized reply rejected before observation/send: %d bytes, question=%s, dispatch=%s", len(serialized), questionID, started.DispatchID)
	if os.Getenv("STEERLINE_REAL_P10_SEND_BOUNDED_REPLY") == "1" {
		requestID, replyErr := remoteWorkerClient(c).Reply(ctx, remoteWorkerRef(ref), questionID, need("STEERLINE_REAL_P10_BOUNDED_REPLY"))
		if replyErr != nil || requestID == "" {
			t.Fatalf("bounded reply to exact home question was not accepted: request=%q error=%v", requestID, replyErr)
		}
		t.Logf("bounded reply accepted: request=%s question=%s dispatch=%s", requestID, questionID, started.DispatchID)
	}
}
