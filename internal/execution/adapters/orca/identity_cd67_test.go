package orcaworker

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWorkerOrcaWorktreeNameIncludesTaskNumberAndRunIdentity(t *testing.T) {
	runID := strings.Repeat("a", 32)
	name, err := WorkerWorktreeName("103", runID)
	if err != nil || name != "sl-103-"+runID {
		t.Fatalf("worktree name = %q / %v", name, err)
	}
	if other, err := WorkerWorktreeName("104", runID); err != nil || other == name {
		t.Fatalf("different tasks shared a worktree name: %q / %v", other, err)
	}
	if other, err := WorkerWorktreeName("103", strings.Repeat("b", 32)); err != nil || other == name {
		t.Fatalf("different runs shared a worktree name: %q / %v", other, err)
	}
	for _, taskID := range []string{"", "0103", "task-103", "../103"} {
		if _, err := WorkerWorktreeName(taskID, runID); err == nil {
			t.Fatalf("invalid task number %q was accepted", taskID)
		}
	}
}

func TestFixedOrcaAgentTerminalRejectsReplacedIncarnation(t *testing.T) {
	title := FixedAgentTitle(strings.Repeat("a", 32))
	receipt, err := json.Marshal(map[string]any{"terminal": map[string]string{
		"handle": "term-original", "incarnationId": "inc-original", "worktreeId": "repo::worktree", "title": title,
	}})
	if err != nil {
		t.Fatal(err)
	}

	live := []Terminal{{Handle: "term-original", IncarnationID: "inc-replacement", WorktreeID: "repo::worktree", Title: "renamed"}}
	if terminal, err := FixedAgentTerminal(live, receipt, title, "repo::worktree"); err == nil || terminal.Handle != "" || !strings.Contains(err.Error(), "incarnation changed") {
		t.Fatalf("replacement terminal was adopted: %+v / %v", terminal, err)
	}
}
