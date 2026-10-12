package app

import (
	"strings"
	"testing"

	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"github.com/erranliu/SteerLine/internal/execution/agentlaunch"
)

func TestAgentLaunchProfileFreezesPreferencesAndDisplaysThem(t *testing.T) {
	base, _ := nativeTaskFixture(t)
	p := testExecutionProfile(t, "orca-high", "Orca high", "orca", "codex", codexworker.ProfileConfig{}, &terminalFixtureProfile{Executable: "orca"})
	p.AgentLaunch = &agentlaunch.Config{Executable: "codex", Model: "gpt-6-luna", ReasoningEffort: "high", Args: []string{"--dangerously-bypass-approvals-and-sandbox"}}
	p = boundProfile(base, p)
	fixed := p.Freeze()
	if err := base.CheckFixedProfile(fixed); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(fixed.ImplementationDisplay, " "), "gpt-6-luna") || !strings.Contains(strings.Join(fixed.ImplementationDisplay, " "), "high") {
		t.Fatal("launch not displayed", fixed)
	}
	p.AgentLaunch.ReasoningEffort = "xhigh"
	p.AgentLaunch.Args[0] = "changed"
	if fixed.AgentLaunch.ReasoningEffort != "high" || fixed.AgentLaunch.Args[0] == "changed" {
		t.Fatal("frozen launch changed")
	}
	if p.Digest() == fixed.Digest {
		t.Fatal("effort edit did not change profile identity")
	}
	selected := base.ExecutionConfig().ApplyFixedExecutionProfile(fixed, base.ExecutionDirectory)
	selected.AgentLaunch.Args[0] = "selected-edit"
	if fixed.AgentLaunch.Args[0] == "selected-edit" {
		t.Fatal("restored launch aliases snapshot")
	}
}
