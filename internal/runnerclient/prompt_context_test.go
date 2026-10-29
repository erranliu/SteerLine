package runnerclient

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"os"
	"strings"
	"testing"
)

func TestPMSWorktreePromptContext(t *testing.T) {
	raw, e := os.ReadFile("testdata/worktree-workflows.json")
	if e != nil {
		t.Fatal(e)
	}
	var choices []runnercontract.WorkflowChoice
	if e = json.Unmarshal(raw, &choices); e != nil {
		t.Fatal(e)
	}
	cat := &Catalog{Capabilities: Capabilities{Workflows: choices}}
	for _, choice := range choices {
		if choice.ID != "development" && choice.ID != "development-audited" {
			continue
		}
		t.Run(choice.ID, func(t *testing.T) {
			snapshot := []byte(`{"status":"ready","plan_path":"task.plan.md","commit":"fixed","definitions":[{"id":"one"}]}`)
			context, err := prompts.TestingContext(snapshot, true)
			if err != nil {
				t.Fatal(err)
			}
			readOnly, err := prompts.TestingReadOnly()
			if err != nil {
				t.Fatal(err)
			}
			in := Workflow{ID: choice.ID, Version: choice.Version, PromptOverrides: map[string]string{"publish": "User publish", "implement.report": "User report"}}
			got, e := cat.WithTestProgressContext(in, snapshot, true)
			if e != nil {
				t.Fatal(e)
			}
			if !strings.HasPrefix(got.PromptOverrides["implement.execute"], choice.States["implement.execute"].Prompt) || !strings.HasSuffix(got.PromptOverrides["implement.execute"], context) {
				t.Fatal("published default lost")
			}
			if choice.ID == "development-audited" && !strings.HasSuffix(got.PromptOverrides["audit.code"], context) {
				t.Fatal("audit context omitted")
			}
			if got.PromptOverrides["publish"] != "User publish\n\n"+readOnly || got.PromptOverrides["implement.report"] != "User report\n\n"+readOnly || len(in.PromptOverrides) != 2 {
				t.Fatal("other prompts or profile mutated")
			}
			for _, state := range choice.States {
				if state.Contract != "publication" && state.Kind != "report" || state.PromptKey == "" {
					continue
				}
				body := got.PromptOverrides[state.PromptKey]
				if !strings.HasSuffix(body, readOnly) || strings.Contains(body, context) {
					t.Fatal("test writer leaked into publication/report turn", state.PromptKey, body)
				}
			}
			in.PromptOverrides["implement.execute"] = "User implementation"
			custom, e := cat.WithTestProgressContext(in, snapshot, true)
			if e != nil || custom.PromptOverrides["implement.execute"] != "User implementation\n\n"+context {
				t.Fatal("configured prompt replaced", e)
			}
		})
	}
	for _, choice := range choices {
		if choice.ID == "development-audited" {
			broken := choice
			broken.States = map[string]runnercontract.WorkflowPrompt{"implement.execute": choice.States["implement.execute"]}
			cat := &Catalog{Capabilities: Capabilities{Workflows: []runnercontract.WorkflowChoice{broken}}}
			if _, e := cat.WithTestProgressContext(Workflow{ID: choice.ID, Version: choice.Version}, []byte(`{"status":"ready"}`), false); e == nil {
				t.Fatal("missing audited prompt accepted")
			}
		}
	}
	incomplete := &Catalog{Capabilities: Capabilities{Workflows: []runnercontract.WorkflowChoice{{ID: "development", Version: 2, PromptSteps: []string{"implement.execute"}}}}}
	if _, e = incomplete.WithTestProgressContext(Workflow{ID: "development", Version: 2}, []byte(`{"status":"ready"}`), false); e == nil {
		t.Fatal("missing published body accepted")
	}
}
