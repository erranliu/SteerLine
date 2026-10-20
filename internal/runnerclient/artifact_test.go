package runnerclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTMEvidencePublishedContractAndBytes(t *testing.T) {
	raw, err := os.ReadFile("testdata/content-capabilities.json")
	if err != nil {
		t.Fatal(err)
	}
	var caps Capabilities
	_ = json.Unmarshal(raw, &caps)
	catalog, err := CompileCatalog(caps)
	if err != nil {
		t.Fatal(err)
	}
	if err = catalog.ValidateEvidence(); err != nil {
		t.Fatal(err)
	}
	catalog.Capabilities.RunEvidence.Commands = nil
	if catalog.ValidateEvidence() == nil {
		t.Fatal("missing R2 accepted")
	}
	exe, _ := os.Executable()
	t.Setenv("STEERLINE_RUNNER_CLI_FIXTURE", "check-response")
	call := filepath.Join(t.TempDir(), "call")
	t.Setenv("STEERLINE_RUNNER_CLI_CALL", call)
	c := Client{Executable: exe, StateDir: t.TempDir(), Timeout: 5 * time.Second}
	body := []byte("{}")
	encoded := base64.StdEncoding.EncodeToString(body)
	for _, tt := range []struct {
		sum, encoded string
		ok           bool
	}{{"44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a", encoded, true}, {strings.Repeat("0", 64), encoded, false}, {strings.Repeat("0", 64), "?", false}} {
		data := map[string]any{"run_id": "run", "artifact_id": "artifact", "relative_path": "first/managed-test.json", "sha256": tt.sum, "size_bytes": 2, "collected_at": time.Now().UTC(), "read_command": "artifacts read", "encoding": "base64", "content_base64": tt.encoded}
		raw, _ := json.Marshal(data)
		t.Setenv("STEERLINE_CHECK_RESPONSE", `{"protocol":"steerrunner-cli/v1","ok":true,"request_id":null,"data":`+string(raw)+`,"error":null}`)
		if _, _, err := c.ReadArtifact(context.Background(), "run", "artifact"); (err == nil) != tt.ok {
			t.Fatal("incorrect integrity acceptance", err)
		}
	}
	argv, _ := os.ReadFile(call)
	if !strings.Contains(string(argv), "artifacts\nread\n--run-id\nrun\n--artifact-id\nartifact") {
		t.Fatal("incorrect public CLI mapping", string(argv))
	}
}
