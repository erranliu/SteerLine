package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestOriginalTerminalRequiresAgentDetectionBeforeQualifiedProcess(t *testing.T) {
	detected := false
	calls := 0
	cfg := Config{CommandTimeout: "2s", Output: func(context.Context, time.Duration, ...string) ([]byte, error) {
		calls++
		agent := ""
		if detected {
			agent = "codex"
		}
		return json.Marshal(map[string]any{"ok": true, "_meta": map[string]string{"runtimeId": "fixed-runtime"}, "result": map[string]any{"terminal": Terminal{Handle: "original-terminal", WorktreeID: "original-worktree", IncarnationID: "original-incarnation", Connected: true, AgentIdentity: agent}}})
	}}
	port := ExecutionLifecycle{client: New(cfg), orca: ProfileConfig{Executable: "fixture-host"}}
	worktree := Worktree{ID: "original-worktree"}
	process, bound, err := port.processForTerminal(context.Background(), "original-terminal", worktree, true)
	if !errors.Is(err, ErrAgentStarting) || bound || process != (executionbackend.ProcessIdentity{}) {
		t.Fatal("unidentified terminal manufactured a qualified agent process")
	}
	detected = true
	process, bound, err = port.processForTerminal(context.Background(), "original-terminal", worktree, true)
	if err != nil || !bound || process.Terminal != "original-terminal" || process.Incarnation != "original-incarnation" || process.RuntimeID != "fixed-runtime" || calls != 2 {
		t.Fatalf("agent detection changed original terminal or failed: %+v %t %v calls=%d", process, bound, err, calls)
	}
}

func TestAgentBindPendingReceiptDoesNotQualifyOriginalHost(t *testing.T) {
	ctx := context.Background()
	base := strings.Repeat("a", 40)
	process := executionbackend.ProcessIdentity{Kind: "orca", Terminal: "original-terminal", RuntimeID: "fixed-runtime", Incarnation: "original-incarnation", WorktreeID: "original-worktree", Executable: "fixture-host"}
	snapshot := executionbackend.LifecycleSnapshot{
		Ref:                 executionbackend.ExecutionRef{Backend: executionbackend.BackendOrca, Protocol: executionbackend.ProtocolOrcaTerminalV2, Target: executionbackend.ExecutionTarget{ID: "fixed-target", WorktreeID: process.WorktreeID}, External: process, BaseCommit: base},
		Session:             executionbackend.SessionContext{ProjectID: stableIDValue(1), WorkflowRunID: stableIDValue(2), ExecutionID: stableIDValue(3), AttemptID: stableIDValue(4), TaskID: "808"},
		ConfigurationDigest: "fixed-profile", ResourceGeneration: stableIDValue(5),
	}
	name, err := WorkerWorktreeName(snapshot.Session.TaskID, snapshot.Session.WorkflowRunID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewExecutionStateEnvelope(ExecutionStateIdentity{ProjectID: snapshot.Session.ProjectID, ExecutionID: snapshot.Session.ExecutionID, AttemptID: snapshot.Session.AttemptID, Generation: snapshot.ResourceGeneration, ConfigDigest: snapshot.ConfigurationDigest, Protocol: snapshot.Ref.Protocol, Process: process}, NativeExecutionState{RepositoryID: "original-repository"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Implementation = []executionbackend.ImplementationDataEnvelope{state}
	detected := false
	port := ExecutionLifecycle{orca: ProfileConfig{Executable: process.Executable}, client: New(Config{Output: func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
		if len(args) > 1 && args[0] == "worktree" && args[1] == "list" {
			return json.Marshal(map[string]any{"ok": true, "result": map[string]any{"worktrees": []Worktree{{ID: process.WorktreeID, RepoID: "original-repository", DisplayName: name, Path: "C:/fixture/worktree", Branch: "fixture/task", Head: base, BaseRef: base}}}})
		}
		agent := ""
		if detected {
			agent = "codex"
		}
		return json.Marshal(map[string]any{"ok": true, "_meta": map[string]string{"runtimeId": process.RuntimeID}, "result": map[string]any{"terminal": Terminal{Handle: process.Terminal, IncarnationID: process.Incarnation, WorktreeID: process.WorktreeID, WorktreePath: "C:/fixture/worktree", Branch: "fixture/task", Connected: true, AgentIdentity: agent}}})
	}})}
	request := executionbackend.LifecycleRequest{Snapshot: snapshot, Operation: executionbackend.Operation{ID: "original-operation", Action: string(executionbackend.LifecycleCreateResource)}}
	receipt := port.bindAgent(ctx, request)
	if receipt.Disposition != executionbackend.OperationAccepted || !receipt.State.ResourcePending || receipt.Process != nil || receipt.OriginalHost == nil || *receipt.OriginalHost != process {
		t.Fatalf("pending detection qualified or replaced original host: %+v", receipt)
	}
	detected = true
	receipt = port.bindAgent(ctx, request)
	if receipt.Disposition != executionbackend.OperationConfirmed || receipt.State.ResourcePending || receipt.Process == nil || *receipt.Process != process {
		t.Fatalf("detected agent did not qualify the same original host: %+v", receipt)
	}
}
