package app

import (
	"encoding/json"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestProjectConfigDecoderPreservesStrictSingleJSONValueAndBOM(t *testing.T) {
	var command executionbackend.Command
	if err := decodeStrictJSON(append([]byte{0xef, 0xbb, 0xbf}, []byte(`{"executable":"git"}`)...), &command); err != nil {
		t.Fatalf("current BOM-prefixed Git command was rejected: %v", err)
	}
	if command.Executable != "git" {
		t.Fatalf("Git command decoded incorrectly: %+v", command)
	}
	for _, input := range []string{
		`{"executable":"git","unknown":true}`,
		`{"executable":"git"} {"executable":"git"}`,
		`{"executable":"git"} null`,
	} {
		if err := decodeStrictJSON([]byte(input), &command); err == nil {
			t.Errorf("strict decoder accepted %s", input)
		}
	}
}

func TestProjectConfigInputDecoderPreservesOmittedDefaults(t *testing.T) {
	input, err := DecodeProjectConfigInput([]byte(`{"repository":"fixture","branch":"main"}`))
	if err != nil || input.Repository != "fixture" || input.Branch != "main" || input.TaskPoll != "10s" || input.CommandTimeout != "30s" || input.ShutdownGrace != "10s" || len(input.ExecutableTypes) == 0 {
		t.Fatal("omitted defaults lost", input, err)
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	again, err := DecodeProjectConfigInput(raw)
	if err != nil || again.CommandTimeout != input.CommandTimeout || len(again.ExecutableTypes) != len(input.ExecutableTypes) {
		t.Fatal("serialized defaults changed", err)
	}
	explicit, err := DecodeProjectConfigInput([]byte(`{"task_poll":""}`))
	if err != nil || explicit.TaskPoll != "" {
		t.Fatal("explicit invalid value replaced by default", err)
	}
	if _, err := DecodeProjectConfigInput([]byte(`{"unknown_project_setting":true}`)); err == nil {
		t.Fatal("unknown project setting accepted")
	}
}
