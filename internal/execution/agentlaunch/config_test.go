package agentlaunch

import (
	"strings"
	"testing"
)

func TestAgentLaunchCompilesAndProtectsPreferences(t *testing.T) {
	for _, effort := range []string{"high", "xhigh"} {
		c := &Config{Executable: "codex", Model: "gpt-6-luna", ReasoningEffort: effort, Args: []string{"--dangerously-bypass-approvals-and-sandbox"}}
		cmd, err := c.Command("codex")
		if err != nil || !strings.Contains(cmd, "'gpt-6-luna'") || !strings.Contains(cmd, "model_reasoning_effort=\""+effort+"\"") || !strings.Contains(cmd, "tui.animations=false") {
			t.Fatal(cmd, err)
		}
		copy := c.Clone()
		copy.Args[0] = "changed"
		if c.Args[0] == "changed" {
			t.Fatal("launch arguments aliased")
		}
	}
	for _, config := range []*Config{
		{Executable: "codex", ReasoningEffort: "high"},
		{Executable: "codex", Model: "chosen", ReasoningEffort: "invalid"},
		{Executable: "codex", Model: "chosen", Args: []string{"--model", "different"}},
		{Executable: "codex", Model: "chosen", Args: []string{"-c", "model_reasoning_effort=\"low\""}},
		{Executable: "codex\nother"},
	} {
		if _, err := config.Command("codex"); err == nil {
			t.Fatal("invalid launch accepted", config)
		}
	}
	c := &Config{Executable: "agent's path", Args: []string{"$secret; other"}}
	cmd, err := c.Command("opaque")
	if err != nil || !strings.Contains(cmd, "'agent''s path'") || !strings.Contains(cmd, "'$secret; other'") {
		t.Fatal("shell quoting changed values", cmd, err)
	}
}
