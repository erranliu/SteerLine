package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline"
)

func TestNativeTaskListCLIInitializesReadOnlyProtocolWithoutBeads(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	repo := filepath.Join(t.TempDir(), "repo")
	remote := filepath.Join(t.TempDir(), "origin.git")
	for _, args := range [][]string{{"init", "--quiet", "-b", "main", repo}, {"init", "--quiet", "--bare", remote}, {"-C", repo, "remote", "add", "origin", remote}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	configPath := filepath.Join(t.TempDir(), "native.json")
	data, err := json.Marshal(map[string]any{"repository": repo, "remote": "origin", "branch": "main", "task_authority": "native", "bd": map[string]any{"executable": "missing-bd"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = write
	callErr := runNativeTaskCommand([]string{"list", "-config", configPath})
	_ = write.Close()
	os.Stdout = oldStdout
	var output bytes.Buffer
	_, readErr := output.ReadFrom(read)
	_ = read.Close()
	if callErr != nil || readErr != nil {
		t.Fatalf("native task list failed without Beads: %v / %v", callErr, readErr)
	}
	var page steerline.NativeTaskPage
	if err = json.Unmarshal(output.Bytes(), &page); err != nil {
		t.Fatalf("invalid task protocol JSON: %v: %s", err, output.String())
	}
	if page.Protocol != "native-tasks/v1" || page.ProjectID == "" || page.SampledAt.IsZero() || len(page.Tasks) != 0 || page.Capabilities.DependencyReadiness != "not_provided" {
		t.Fatalf("unexpected native task list: %+v", page)
	}
	if err = runNativeTaskCommand([]string{"show", "-task-id", "01", "-config", configPath}); err == nil || !strings.Contains(err.Error(), "contract_mismatch") {
		t.Fatalf("noncanonical task ID was accepted: %v", err)
	}
}
