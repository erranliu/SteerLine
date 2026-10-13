package orca

import (
	"encoding/json"
	"testing"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestOrcaNativeStateIdentity(t *testing.T) {
	p := b.ProcessIdentity{WorktreeID: "worktree", Terminal: "terminal", Incarnation: "original"}
	for _, incarnation := range []string{"", "original", "replacement"} {
		raw, _ := json.Marshal(map[string]any{"worktree": "worktree", "tabs": []any{map[string]string{"terminal": "terminal", "incarnationId": incarnation, "parentTabId": "tab", "leafId": "leaf"}}})
		s, err := DecodeOriginalState(raw, p)
		if (err == nil) != (incarnation != "replacement") || err == nil && s.PaneKey != "tab:leaf" {
			t.Fatalf("stream metadata binding %q: %+v %v", incarnation, s, err)
		}
	}
	for _, parent := range []string{"", "other-pane"} {
		raw, _ := json.Marshal(map[string]any{"worktrees": []any{map[string]any{"worktreeId": "worktree", "agents": []any{map[string]string{"paneKey": "tab:leaf", "parentPaneKey": parent}}}}})
		_, err := DecodeOriginalAgent(raw, "worktree", "tab:leaf")
		if (err == nil) != (parent == "") {
			t.Fatal("child main-agent row was accepted", err)
		}
		if _, err := DecodeOriginalAgent(raw, "other-worktree", "tab:leaf"); err == nil {
			t.Fatal("query selected another worktree")
		}
		if _, err := DecodeOriginalAgent(raw, "worktree", "other-pane"); err == nil {
			t.Fatal("query selected another pane")
		}
	}
}
