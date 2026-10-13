package codexworker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestRecoveryProcessIdentityRequiresPositiveNativeProof(t *testing.T) {
	for _, status := range []string{"matching process still exists", "process absent", "process identity mismatch", "process inspection unavailable"} {
		t.Run(status, func(t *testing.T) {
			root := t.TempDir()
			process := b.ProcessIdentity{Kind: "os", PID: 42, Created: "original"}
			binding := Binding{Process: &process, Endpoint: "ws://127.0.0.1:1", Generation: "original-generation"}
			raw, err := json.Marshal(ProcessReceipt{Managed: true, Identity: process, Endpoint: binding.Endpoint, Generation: binding.Generation})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "process.json"), raw, 0600); err != nil {
				t.Fatal(err)
			}
			backend := Backend{Config: Config{ServerDir: root}, Process: ProcessPort{Inspect: func(*b.ProcessIdentity) string { return status }}}
			managed, absent := backend.ManagedProcessState(binding)
			if managed != (status == "matching process still exists" || status == "process absent") || absent != (status == "process absent") {
				t.Fatalf("ambiguous identity became a usable resource: %v/%v", managed, absent)
			}
		})
	}
}

func TestRecoveryHistoryRejectsChangedFrozenBindingBeforeNativeRead(t *testing.T) {
	workspace, home := t.TempDir(), t.TempDir()
	t.Setenv("CODEX_HOME", home)
	session := "00000000-0000-0000-0000-000000000001"
	base := historyFixtureBinding(workspace, strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32), session)
	base.Home = home
	for _, tc := range []struct {
		name   string
		change func(*HistoryBinding)
	}{
		{"project", func(h *HistoryBinding) { h.ProjectID = "another-project" }},
		{"execution", func(h *HistoryBinding) { h.ExecutionID = strings.Repeat("d", 32) }},
		{"attempt", func(h *HistoryBinding) { h.AttemptID = strings.Repeat("d", 32) }},
		{"workflow", func(h *HistoryBinding) { h.WorkflowID = strings.Repeat("d", 32) }},
		{"thread", func(h *HistoryBinding) { h.SessionID = "00000000-0000-0000-0000-000000000002" }},
		{"workspace", func(h *HistoryBinding) { h.Workspace = t.TempDir() }},
		{"home", func(h *HistoryBinding) { h.Home = t.TempDir() }},
		{"executable", func(h *HistoryBinding) { h.Command.Executable = "different-codex" }},
		{"arguments", func(h *HistoryBinding) { h.Command.Args = []string{"different-args"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			lifecycle := ExecutionLifecycle{HistoryCommand: base.Command}
			lifecycle.Backend.Config.RunDir = root
			snapshot := b.LifecycleSnapshot{Ref: b.ExecutionRef{WorkspacePath: workspace}, Session: b.SessionContext{ProjectID: base.ProjectID, ExecutionID: base.ExecutionID, AttemptID: base.AttemptID, WorkflowRunID: base.WorkflowID, CurrentCommandID: strings.Repeat("e", 32)}}
			path := localHistoryBindingPath(filepath.Join(root, "runs", base.WorkflowID, "turns", snapshot.Session.CurrentCommandID), session)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			changed := base
			tc.change(&changed)
			if err := WriteHistoryBinding(path, changed, func(path string, data []byte) error { return os.WriteFile(path, data, 0600) }); err != nil {
				t.Fatal(err)
			}
			// There is deliberately no native process capability. A frozen-binding
			// rejection must precede any attempt to open a public history server.
			_, err := lifecycle.readRecoveryThread(context.Background(), snapshot, Binding{SessionID: session})
			if err == nil || !strings.Contains(err.Error(), "binding differs") {
				t.Fatalf("changed binding reached native read: %v", err)
			}
		})
	}
}

func TestRecoveryHistoryIdleRequiresOriginalStoppedTurn(t *testing.T) {
	ended := int64(1)
	thread := HistoryThread{Turns: []HistoryTurn{{ID: "original", Status: "inProgress"}}}
	if historyThreadIdle(thread, "") || !historyThreadIdle(thread, "original") {
		t.Fatal("writer proof did not constrain the original interrupted turn")
	}
	for _, status := range []string{"completed", "failed", "interrupted"} {
		thread.Turns[0].Status = status
		if historyThreadIdle(thread, "") || !historyThreadIdle(thread, "original") {
			t.Fatal("nullable original completion timing hid proven stopped ownership")
		}
	}
	thread.Turns[0].Status = "inProgress"
	thread.Turns = append(thread.Turns, HistoryTurn{ID: "manual", Status: "inProgress"})
	if historyThreadIdle(thread, "original") {
		t.Fatal("old worker death hid a live manual turn")
	}
	thread.Turns[1].Status, thread.Turns[1].CompletedAt = "completed", &ended
	if !historyThreadIdle(thread, "original") {
		t.Fatal("completed manual result could not be reverified")
	}
	thread.Turns[1].CompletedAt = nil
	if historyThreadIdle(thread, "original") {
		t.Fatal("missing completion proof treated as idle")
	}
}
