package codexworker

import (
	"testing"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func TestCD63CodecIdentityAndVersion(t *testing.T) {
	t.Run("shared_question_envelope", TestCD62NativeFactsAndQuestionRoute)
	payload := []byte(`{"executable":"codex","args":["--search"],"model":"gpt-6-luna","reasoning_effort":"high"}`)
	settings, err := DecodeProfileConfig(payload)
	if err != nil || settings.Executable != "codex" || settings.Args[0] != "--search" {
		t.Fatalf("valid profile payload did not decode: %+v / %v", settings, err)
	}
	policy := executionapi.ExecutionPolicy{Filesystem: executionapi.FilesystemPolicy{ReadScope: "host", WriteScope: "workspace", GitMetadataWrite: true}, Network: "allow", Escalation: "deny"}
	normalized, err := NormalizeProfileConfig(settings, policy, "windows", "local", "codex")
	if err != nil || normalized.WindowsSandbox != "elevated" || normalized.ReasoningEffort != "high" {
		t.Fatalf("implementation defaults were not normalized: %+v / %v", normalized, err)
	}
	if _, err := DecodeProfileConfig([]byte(`{"executable":"codex","args":[],"sandbox":"danger-full-access"}`)); err == nil {
		t.Fatal("unknown native field was accepted")
	}
	if _, err := NormalizeProfileConfig(settings, policy, "windows", "orca_supervised", "codex"); err == nil {
		t.Fatal("unsupported remote native permission mapping was inferred")
	}
}
