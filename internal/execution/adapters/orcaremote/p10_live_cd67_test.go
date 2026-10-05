package orca_remote_worker_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	remote "github.com/erranliu/SteerLine/internal/execution/adapters/orcaremote"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

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
	settings := remote.Settings{ConnectionID: need("STEERLINE_REAL_P10_CONNECTION_ID"), EnvironmentID: need("STEERLINE_REAL_P10_ENVIRONMENT_ID"), RepositorySelector: need("STEERLINE_REAL_P10_REPOSITORY_SELECTOR"), CoordinatorTerminal: need("STEERLINE_REAL_P10_COORDINATOR_TERMINAL"), PollInterval: "2s"}
	c := remote.Config{Agent: "codex", Model: "gpt-6-luna", ReasoningEffort: "high", CommandTimeout: "30s", Settings: settings, Call: orcaworker.New(orcaworker.Config{CommandTimeout: "30s", Output: func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return exec.CommandContext(ctx, "orca", args...).Output()
	}}).Raw}

	started := remote.StartReceipt{
		RunID:           need("STEERLINE_REAL_P10_RUN_ID"),
		TaskID:          need("STEERLINE_REAL_P10_TASK_ID"),
		DispatchID:      need("STEERLINE_REAL_P10_DISPATCH_ID"),
		WorktreeID:      need("STEERLINE_REAL_P10_WORKTREE_ID"),
		CenterRuntimeID: need("STEERLINE_REAL_P10_CENTER_RUNTIME_ID"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	observed, err := remote.New(c).Show(ctx, started)
	if err != nil || observed.Liveness != "live" {
		t.Fatalf("exact home worker is not live: observation=%+v error=%v", observed, err)
	}
	target := targetFor(c)
	target.OrchestrationRun = started.RunID
	target.ExternalTask = started.TaskID
	target.Dispatch = started.DispatchID
	target.ExecutionHost = observed.Identity.ExecutionHost
	target.WorktreeID = started.WorktreeID
	ref := executionbackend.ExecutionRef{RunID: need("STEERLINE_REAL_P10_WORKFLOW_RUN_ID"), Version: executionbackend.ExecutionRecordVersion, Backend: executionbackend.BackendOrca, Protocol: executionbackend.ProtocolOrcaSupervised, Target: target, External: observed.Identity}
	questionID := need("STEERLINE_REAL_P10_QUESTION_ID")
	oversized := "x" + strings.Repeat("\n", 4200)
	serialized, err := json.Marshal(struct {
		QuestionID string `json:"question_id"`
		Body       string `json:"body"`
		Run        string `json:"run"`
		From       string `json:"from"`
		Dispatch   string `json:"dispatch"`
	}{questionID, oversized, started.RunID, c.Settings.CoordinatorTerminal, started.DispatchID})
	if err != nil || len(serialized) <= remote.MaxMessageBytes {
		t.Fatalf("probe answer did not exceed the serialized limit: bytes=%d error=%v", len(serialized), err)
	}
	if _, err = remote.New(c).Reply(ctx, remote.FromExecutionRef(ref), questionID, oversized); !isBackendError(identityError(err), executionbackend.BackendRejected) || !strings.Contains(err.Error(), "serialized message exceeds") {
		t.Fatalf("oversized answer did not fail before Orca send: %v", err)
	}
	t.Logf("oversized serialized reply rejected before observation/send: %d bytes, question=%s, dispatch=%s", len(serialized), questionID, started.DispatchID)
	if os.Getenv("STEERLINE_REAL_P10_SEND_BOUNDED_REPLY") == "1" {
		requestID, replyErr := remote.New(c).Reply(ctx, remote.FromExecutionRef(ref), questionID, need("STEERLINE_REAL_P10_BOUNDED_REPLY"))
		if replyErr != nil || requestID == "" {
			t.Fatalf("bounded reply to exact home question was not accepted: request=%q error=%v", requestID, replyErr)
		}
		t.Logf("bounded reply accepted: request=%s question=%s dispatch=%s", requestID, questionID, started.DispatchID)
	}
}
