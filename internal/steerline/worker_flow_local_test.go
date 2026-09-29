package steerline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const workerFlowMockSession = "00000000-0000-0000-0000-000000000008"

func TestWorkerFlowMockCodexHelper(t *testing.T) {
	if os.Getenv("STEERLINE_WORKER_FLOW_MOCK") != "1" {
		return
	}
	args := strings.Join(os.Args, " ")
	if strings.Contains(args, " resume ") && !strings.Contains(args, " resume "+workerFlowMockSession+" ") {
		os.Exit(2)
	}
	if !strings.Contains(args, " resume ") && strings.Contains(args, workerFlowMockSession) {
		os.Exit(3)
	}
	_, _ = fmt.Fprintln(os.Stdout, `{"type":"thread.started","thread_id":"`+workerFlowMockSession+`"}`)
	_, _ = fmt.Fprintln(os.Stdout, `{"type":"turn.started"}`)
	_, _ = fmt.Fprintln(os.Stdout, `{"type":"item.completed","item":{"id":"item-1","type":"agent_message","text":"turn done"}}`)
	_, _ = fmt.Fprintln(os.Stdout, `{"type":"turn.completed"}`)
	os.Exit(0)
}

func TestLocalWorkerTurnResumesExactSessionInSameWorkspace(t *testing.T) {
	t.Setenv("STEERLINE_WORKER_FLOW_MOCK", "1")
	c, base := nativeTaskFixture(t)
	if err := c.ResolveIdentity(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.WorkflowProtocol, c.Host, c.Agent, c.Sandbox = WorkerFlowVersion, "local", "codex", "workspace-write"
	c.Codex = Command{Executable: os.Args[0], Args: []string{"-test.run=^TestWorkerFlowMockCodexHelper$", "--"}}
	c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	if err := c.prepareWorktreeRoot(); err != nil {
		t.Fatal(err)
	}
	run := &Run{TaskID: "8", RunID: strings.Repeat("a", 32), AttemptID: strings.Repeat("b", 32), ExecutionID: strings.Repeat("c", 32), WorkflowVersion: WorkerFlowVersion, Target: ExecutionTarget{ID: "local"}}
	intent, err := planWorkspace(c, run.TaskID, run.RunID, base)
	if err != nil {
		t.Fatal(err)
	}
	run.WorkspaceIntent = intent
	if err := prepareWorktreeOnly(context.Background(), c, run); err != nil {
		t.Fatal(err)
	}
	run.WorktreePath, run.WorktreeBranch = ptr(intent.Path), ptr(intent.Branch)
	execution := &ExecutionRecord{ExecutionID: run.ExecutionID, WorkflowRunID: run.RunID, Workspace: WorkspaceEvidence{Worktree: intent.Path, Branch: intent.Branch}}
	for number := 1; number <= 2; number++ {
		turnID := strings.Repeat(fmt.Sprint(number), 32)
		worker, path, err := startLocalWorkerTurn(context.Background(), c, run, execution, turnID, "continue")
		if err != nil {
			if errors.Is(err, errDetachUnsupported) && os.Getenv("STEERLINE_REQUIRE_DETACHED_TESTS") != "1" {
				t.Skipf("host cannot retain the local worker across service exit: %v", err)
			}
			t.Fatal(err)
		}
		var observedSession string
		for event := range worker.Events {
			if event.Session != "" {
				observedSession = event.Session
			}
			if event.Persisted != nil {
				event.Persisted <- nil
			}
		}
		if err := <-worker.Done; err != nil {
			t.Fatal(err)
		}
		if observedSession != workerFlowMockSession || filepath.Base(path) != "events.jsonl" {
			t.Fatalf("turn %d lost session/log identity: %s %s", number, observedSession, path)
		}
		outcome, err := readWorkerTurnOutcome(path)
		if err != nil || !outcome.Started || !outcome.Completed || outcome.SessionID != workerFlowMockSession || outcome.LastMessage != "turn done" {
			t.Fatalf("turn %d has incomplete observed outcome: %+v %v", number, outcome, err)
		}
		if number == 1 {
			execution.SessionID = observedSession
		}
	}
}
