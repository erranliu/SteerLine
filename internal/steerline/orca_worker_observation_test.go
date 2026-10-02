package steerline

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func TestWorkerOrcaWorktreeNameIncludesTaskNumberAndRunIdentity(t *testing.T) {
	runID := strings.Repeat("a", 32)
	name, err := orcaworker.WorkerWorktreeName("103", runID)
	if err != nil || name != "sl-103-"+runID {
		t.Fatalf("worktree name = %q / %v", name, err)
	}
	if other, err := orcaworker.WorkerWorktreeName("104", runID); err != nil || other == name {
		t.Fatalf("different tasks shared a worktree name: %q / %v", other, err)
	}
	if other, err := orcaworker.WorkerWorktreeName("103", strings.Repeat("b", 32)); err != nil || other == name {
		t.Fatalf("different runs shared a worktree name: %q / %v", other, err)
	}
	for _, taskID := range []string{"", "0103", "task-103", "../103"} {
		if _, err := orcaworker.WorkerWorktreeName(taskID, runID); err == nil {
			t.Fatalf("invalid task number %q was accepted", taskID)
		}
	}
}

func TestNativeOrcaAgentSelectionUsesFrozenProfile(t *testing.T) {
	base := Config{Host: "orca", Agent: "codex", ExecutionPolicy: workspacePolicyFixture()}
	for _, agent := range []string{"codex", "opencode"} {
		codex := codexworker.ProfileConfig{Executable: "codex.exe"}
		orca := &orcaworker.ProfileConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "2m"}
		if agent == "codex" {
			codex.WindowsSandbox = "unelevated"
			orca.Shell = "powershell.exe"
		}
		profile := testExecutionProfile(t, "orca-"+agent, "Orca "+agent, "orca", agent, codex, orca)
		profile = base.bindProjectPolicy(profile)
		base.ExecutionProfiles = []ExecutionProfile{profile}
		if err := validateExecutionProfiles(base); err != nil {
			t.Fatal(err)
		}
		frozen := profile.Freeze()
		if !verifyFixedExecutionProfile(frozen) {
			t.Fatal("profile digest changed while freezing")
		}
		selected := base.ApplyFixedExecutionProfile(frozen)
		if selected.agentName() != agent || selected.needsCodexExecutable() != (agent == "codex") {
			t.Fatalf("Orca agent selection was tied to Codex: %+v", selected)
		}
	}
}

func TestFixedOrcaAgentTerminalRejectsReplacedIncarnation(t *testing.T) {
	title := orcaworker.FixedAgentTitle(strings.Repeat("a", 32))
	receipt, err := json.Marshal(map[string]any{"terminal": map[string]string{
		"handle": "term-original", "incarnationId": "inc-original", "worktreeId": "repo::worktree", "title": title,
	}})
	if err != nil {
		t.Fatal(err)
	}
	op := &OperationRecord{Receipt: receipt}
	live := []orcaTerminal{{Handle: "term-original", IncarnationID: "inc-replacement", WorktreeID: "repo::worktree", Title: "renamed"}}
	if terminal, err := orcaworker.FixedAgentTerminal(live, op.Receipt, title, "repo::worktree"); err == nil || terminal.Handle != "" || !strings.Contains(err.Error(), "incarnation changed") {
		t.Fatalf("replacement terminal was adopted: %+v / %v", terminal, err)
	}
}
