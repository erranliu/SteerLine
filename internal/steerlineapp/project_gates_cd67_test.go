package steerlineapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkerFlowConfigRejectsOldProjectGates(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	var base map[string]any
	require(t, json.Unmarshal(projectConfigJSON(t, c), &base))

	write := func(value map[string]any) (Config, error) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "config.json")
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return LoadConfig(path)
	}
	loaded, err := write(base)
	if err != nil || loaded.WorkflowProtocol != WorkerFlowVersion {
		t.Fatalf("new format was not accepted: config=%+v error=%v", loaded, err)
	}
	for name, field := range map[string]any{
		"workflow_definition":    map[string]any{"id": "minimal-v1"},
		"project_steps":          map[string]any{},
		"workspace_dirty_ignore": map[string]any{"paths": []string{"*.meta"}},
	} {
		t.Run(name, func(t *testing.T) {
			value := map[string]any{}
			for k, v := range base {
				value[k] = v
			}
			value[name] = field
			if _, err := write(value); err == nil {
				t.Fatal("obsolete project work gate was accepted")
			}
		})
	}
}
