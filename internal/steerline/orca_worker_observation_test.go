package steerline

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func TestWorkerOrcaWorktreeNameIncludesTaskNumberAndRunIdentity(t *testing.T) {
	runID := strings.Repeat("a", 32)
	name, err := workerOrcaWorktreeName("103", runID)
	if err != nil || name != "sl-103-"+runID {
		t.Fatalf("worktree name = %q / %v", name, err)
	}
	if other, err := workerOrcaWorktreeName("104", runID); err != nil || other == name {
		t.Fatalf("different tasks shared a worktree name: %q / %v", other, err)
	}
	if other, err := workerOrcaWorktreeName("103", strings.Repeat("b", 32)); err != nil || other == name {
		t.Fatalf("different runs shared a worktree name: %q / %v", other, err)
	}
	for _, taskID := range []string{"", "0103", "task-103", "../103"} {
		if _, err := workerOrcaWorktreeName(taskID, runID); err == nil {
			t.Fatalf("invalid task number %q was accepted", taskID)
		}
	}
}

func TestNativeOrcaAgentSelectionUsesFrozenProfile(t *testing.T) {
	base := Config{Host: "orca", Agent: "codex", Sandbox: "workspace-write"}
	for _, agent := range []string{"codex", "opencode"} {
		profile := ExecutionProfile{ID: "orca-" + agent, Name: "Orca " + agent, Host: "orca", Agent: agent, Sandbox: "workspace-write", Orca: OrcaConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "2m"}}
		if agent == "codex" {
			profile.WindowsSandbox = "unelevated"
			profile.Codex = Command{Executable: "codex.exe"}
			profile.Orca.Shell = "powershell.exe"
		}
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
	title := nativeOrcaFixedAgentTitle(strings.Repeat("a", 32))
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
