package runnerclient

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestTMGeneratePublishedContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/content-capabilities.json")
	if err != nil {
		t.Fatal(err)
	}
	var caps Capabilities
	if err = json.Unmarshal(raw, &caps); err != nil {
		t.Fatal(err)
	}
	c, err := CompileCatalog(caps)
	if err != nil {
		t.Fatal(err)
	}
	profile := ExecutionInput{Adapter: "codex-app-server", Target: ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"model":"chosen-model","reasoning_effort":"medium","access_mode":"full_access","github_auth":{"ignored":true}}`)}
	execution, err := GenerationExecution(profile)
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]any
	_ = json.Unmarshal(execution.Settings, &values)
	if values["access_mode"] != "read_only" || values["model"] != "chosen-model" || len(values) != 3 {
		t.Fatalf("unsafe inherited settings %s", execution.Settings)
	}
	text := "# Plan"
	input := runnercontract.GenerateRunInput{Title: "test", Workflow: Workflow{ID: "generate", Version: 1}, Execution: execution, Generate: runnercontract.GenerateInput{Instruction: "select tests", Materials: []Document{{Kind: "inline", Path: "plan.md", Content: &text}}, OutputSchema: json.RawMessage(`{"type":"object"}`)}}
	if err = c.ValidateGenerate(input); err != nil {
		t.Fatal(err)
	}
	input.Execution.Settings = json.RawMessage(`{"access_mode":"full_access"}`)
	if err = c.ValidateGenerate(input); err == nil {
		t.Fatal("full access generation admitted")
	}
	profile.Target.Kind = "orca_environment"
	if _, err = GenerationExecution(profile); err == nil {
		t.Fatal("remote profile used")
	}
	c.generate = nil
	if err = c.ValidateGenerate(input); err == nil {
		t.Fatal("missing capability accepted")
	}
}
