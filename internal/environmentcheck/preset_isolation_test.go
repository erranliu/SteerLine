package environmentcheck_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestRunnerCheckPresetIsolation(t *testing.T) {
	s, _, store := setup(t)
	s.Profiles[0].Execution.Settings = json.RawMessage(`{"executable":"codex","model":"preset-model","access_mode":"full_access"}`)
	before, _ := json.Marshal(s.Profiles)
	q := start()
	rec, err := s.Submit(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.CheckRequest(context.Background(), s.Project, q.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	raw := stored.Operations[0].Request.Input
	if bytes.Contains(raw, []byte(`"workflow"`)) || bytes.Contains(raw, []byte(`"audit_model"`)) {
		t.Fatalf("workflow leaked to check: %s", raw)
	}
	var settings map[string]any
	if err := json.Unmarshal(rec.Input.Execution.Settings, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["model"] != "preset-model" || settings["access_mode"] != "sandbox" {
		t.Fatal("check override lost preset values", settings)
	}
	after, _ := json.Marshal(s.Profiles)
	if !bytes.Equal(before, after) {
		t.Fatal("check modified shared preset", string(after))
	}
}
