package codexworker

import (
	"testing"
)

func TestCD63CodecIdentityAndVersion(t *testing.T) {
	t.Run("binding endpoint validated before connection", func(t *testing.T) {
		identity := AppServerBindingIdentity{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Generation: "resources", ConfigDigest: "snapshot"}
		for _, endpoint := range []string{"ws://outside.example:80", "ws://127.0.0.1:0", "ws://127.0.0.1:70000"} {
			if _, err := NewAppServerBindingEnvelope(identity, AppServerBinding{Endpoint: endpoint, Generation: "native"}); err == nil {
				t.Fatalf("invalid endpoint accepted: %s", endpoint)
			}
		}
	})
	payload := []byte(`{"executable":"codex","model":"gpt-6-luna","reasoning_effort":"high"}`)
	settings, err := DecodeProfileConfig(payload)
	if err != nil || settings.Executable != "codex" {
		t.Fatalf("valid profile payload did not decode: %+v / %v", settings, err)
	}
	normalized, err := NormalizeProfileConfig(settings, "local", "codex")
	if err != nil || normalized.ReasoningEffort != "high" {
		t.Fatalf("implementation defaults were not normalized: %+v / %v", normalized, err)
	}
	if _, err := DecodeProfileConfig([]byte(`{"executable":"codex","args":[],"sandbox":"danger-full-access"}`)); err == nil {
		t.Fatal("unknown native field was accepted")
	}
	if _, err := NativePermissions("windows", "orca_supervised", "codex"); err == nil {
		t.Fatal("unsupported remote native permission mapping was inferred")
	}
	if _, _, err := NormalizeProfilePayload([]byte(`{"executable":"codex","model":" bad-model "}`), "windows", "orca_supervised", "codex"); err == nil {
		t.Fatal("remote profile skipped the implementation's model validation")
	}
}
