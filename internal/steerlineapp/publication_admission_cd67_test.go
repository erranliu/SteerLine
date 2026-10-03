package steerlineapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectPublicationPolicyRequiredBeforeRepositoryProbe(t *testing.T) {
	for _, policy := range []any{nil, map[string]string{}, map[string]string{"publisher": "automatic"}} {
		path := filepath.Join(t.TempDir(), "project.json")
		raw, err := json.Marshal(map[string]any{"publication_policy": policy, "execution_policy": workspacePolicyFixture(), "repository": "must-not-be-probed", "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion, "git": map[string]string{"executable": "must-not-be-called"}})
		require(t, err)
		require(t, os.WriteFile(path, raw, 0600))
		_, err = LoadConfig(path)
		if err == nil || !strings.Contains(err.Error(), "publication_policy.publisher") {
			t.Fatalf("missing/invalid policy reached repository probe: %v", err)
		}
	}
}
