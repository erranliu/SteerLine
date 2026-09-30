package steerline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

// Set STEERLINE_ORCA_WORKER_TERMINAL and, for a paired host,
// STEERLINE_ORCA_WORKER_ENVIRONMENT to run this against an isolated Orca
// worktree. The ordinary Go suite never sends a live agent prompt.
func TestWorkerFlowOrcaTurnAdapterLive(t *testing.T) {
	handle := os.Getenv("STEERLINE_ORCA_WORKER_TERMINAL")
	if handle == "" {
		t.Skip("live Orca worker terminal was not selected")
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	c := Config{Repository: cwd, RunDir: t.TempDir(), CommandTimeout: "30s", Sandbox: "read-only", Agent: "codex", Host: "orca", Codex: Command{Executable: "codex"}, Orca: OrcaConfig{Executable: "orca", Shell: "cmd.exe"}}
	if env := os.Getenv("STEERLINE_ORCA_WORKER_ENVIRONMENT"); env != "" {
		c.Host = "orca_supervised"
		c.Orca.Supervised = &SupervisedOrcaConfig{EnvironmentID: env}
	}
	c.identity = "orca-worker-adapter-live-probe"
	ctx := context.Background()
	term, err := showWorkerOrcaTerminal(ctx, c, handle)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.FromSlash(term.WorktreePath)
	branch := strings.TrimPrefix(term.Branch, "refs/heads/")
	identity := workerOrcaIdentity(term, c)
	run := &Run{RunID: strings.Repeat("a", 32), ExecutionID: strings.Repeat("b", 32), AttemptID: strings.Repeat("c", 32), WorkflowVersion: WorkerFlowVersion, Backend: string(executionapi.BackendOrca), Protocol: string(executionapi.ProtocolOrcaTerminalV2), Agent: "codex", Target: executionapi.ExecutionTarget{ID: "orca-live-probe"}, WorktreePath: &path, WorktreeBranch: &branch, Process: &identity}
	if c.hostName() == "orca_supervised" {
		run.Protocol = string(executionapi.ProtocolOrcaSupervised)
	}
	execution := &ExecutionRecord{ExecutionID: run.ExecutionID, WorkflowRunID: run.RunID, Process: &identity, Workspace: WorkspaceEvidence{Worktree: path, Branch: branch}}
	for i, marker := range []string{"WF_ORCA_ADAPTER_FIRST_OK", "WF_ORCA_ADAPTER_FEEDBACK_OK"} {
		turnID := strings.Repeat(string(rune('1'+i)), 32)
		worker, events, _, err := startWorkerOrcaTurn(ctx, c, run, execution, turnID, "Reply only "+marker)
		if err != nil {
			t.Fatal(err)
		}
		for event := range worker.Events {
			if event.Persisted != nil {
				event.Persisted <- nil
			}
		}
		if err := <-worker.Done; err != nil {
			t.Fatal(err)
		}
		outcome, err := codexworker.ReadExecOutcome(events)
		if err != nil || !outcome.Started || !outcome.Completed || outcome.LastMessage != marker || !codexThreadID.MatchString(outcome.SessionID) {
			t.Fatalf("Orca worker turn %d did not complete on the original session: outcome=%+v err=%v", i+1, outcome, err)
		}
		if i == 0 {
			execution.SessionID = outcome.SessionID
		} else if execution.SessionID != outcome.SessionID {
			t.Fatal("Orca feedback replaced the native Codex session")
		}
	}
}
