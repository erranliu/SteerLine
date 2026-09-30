package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline"
)

func TestNativeTaskBatchCLIValidatesCreatesAndDiscardsFixedTaskIDs(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	global, err := json.Marshal(map[string]any{
		"execution_profiles":        []map[string]any{{"id": "local", "name": "Local Codex", "host": "local", "agent": "codex", "sandbox": "workspace-write", "codex": map[string]string{"executable": "codex"}}},
		"default_execution_profile": "local",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(steerline.GlobalExecutionConfigPath(), global, 0600); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(t.TempDir(), "repo")
	remoteURL := "https://forge.invalid/org/repo.git"
	for _, args := range [][]string{{"init", "--quiet", "-b", "main", repo}, {"-C", repo, "remote", "add", "origin", remoteURL}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	files := map[string]string{"docs/spec/example.spec.md": "# Example Spec\n\n## Acceptance\n\nPass.\n", "docs/spec/example.plan.md": "# Example Plan\n", "docs/spec/native-task-constraints.md": "# Shared Constraints\n"}
	for name, body := range files {
		path := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"-C", repo, "add", "."}, {"-C", repo, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "task batch fixture"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	head, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.TrimSpace(string(head))
	configPath := filepath.Join(t.TempDir(), "native.json")
	configJSON, _ := json.Marshal(map[string]any{"repository": repo, "remote": "origin", "branch": "main", "task_authority": "native", "workflow_protocol": steerline.WorkerFlowVersion, "executable_types": []string{"task"}})
	if err = os.WriteFile(configPath, configJSON, 0600); err != nil {
		t.Fatal(err)
	}
	c, err := steerline.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := steerline.OpenNativeTaskProject(context.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	ref := func(name string) steerline.FileReference {
		blob, e := exec.Command("git", "-C", repo, "cat-file", "blob", commit+":"+name).Output()
		if e != nil {
			t.Fatal(e)
		}
		sum := sha256.Sum256(blob)
		return steerline.FileReference{Path: name, Commit: commit, SHA256: hex.EncodeToString(sum[:])}
	}
	contract := steerline.TaskContract{ContractID: "contract-example", Repository: remoteURL, Remote: "origin", Branch: "main", SpecRef: ref("docs/spec/example.spec.md"), PlanRef: ref("docs/spec/example.plan.md"), CommonRefs: []steerline.FileReference{ref("docs/spec/native-task-constraints.md")}}
	contract.AcceptanceRefs = []steerline.FileReference{contract.SpecRef}
	contract.Digest, err = steerline.TaskContractDigest(contract)
	if err != nil {
		t.Fatal(err)
	}
	contract2 := contract
	contract2.ContractID = "contract-second"
	contract2.Digest, err = steerline.TaskContractDigest(contract2)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(steerline.TaskBatchInput{SchemaVersion: "task-batch/v1", ProjectID: c.Identity(), Tasks: []steerline.TaskBatchTask{{LocalKey: "fixed-task", TaskID: 70, Title: "Fixed planned task", Type: "task", Contract: contract}, {LocalKey: "second-task", TaskID: 71, Title: "Second task", Type: "task", Contract: contract2}}, Dependencies: []steerline.TaskBatchEdge{}})
	if err != nil {
		t.Fatal(err)
	}
	batchPath := filepath.Join(t.TempDir(), "batch.json")
	if err = os.WriteFile(batchPath, payload, 0600); err != nil {
		t.Fatal(err)
	}
	validationOutput, callErr := captureTaskCLI(t, []string{"validate", "-file", batchPath, "-config", configPath})
	if callErr != nil {
		t.Fatalf("validate failed: %v: %s", callErr, validationOutput)
	}
	var validation steerline.TaskBatchValidation
	if err = json.Unmarshal(validationOutput, &validation); err != nil || !validation.Valid || validation.AuthorityVersion == 0 {
		t.Fatalf("unexpected validation output: %v: %s", err, validationOutput)
	}
	requestID := strings.Repeat("1", 32)
	createOutput, callErr := captureTaskCLI(t, []string{"create", "-file", batchPath, "-request-id", requestID, "-expected-authority-version", fmt.Sprint(validation.AuthorityVersion), "-by", "planner", "-config", configPath})
	if callErr != nil {
		t.Fatalf("create failed: %v: %s", callErr, createOutput)
	}
	var receipt steerline.TaskBatchReceipt
	if err = json.Unmarshal(createOutput, &receipt); err != nil || receipt.TaskIDs["fixed-task"] != "70" || !strings.Contains(receipt.DispatchNote, "does not start work") {
		t.Fatalf("unexpected create receipt: %v: %+v", err, receipt)
	}
	addOutput, callErr := captureTaskCLI(t, []string{"add-predecessors", "-task-id", "71", "-predecessors", "70", "-request-id", strings.Repeat("3", 32), "-expected-version", "1", "-expected-graph-revision", "1", "-reason", "second task now requires first", "-by", "planner", "-config", configPath})
	if callErr != nil {
		t.Fatalf("add-predecessors failed: %v: %s", callErr, addOutput)
	}
	var added steerline.TaskPredecessorAddReceipt
	if err = json.Unmarshal(addOutput, &added); err != nil || added.TaskVersion != 2 || added.GraphRevision != 2 || added.TaskID != "71" {
		t.Fatalf("unexpected predecessor receipt: %v: %+v", err, added)
	}
	discardOutput, callErr := captureTaskCLI(t, []string{"discard", "-task-id", "70", "-request-id", strings.Repeat("2", 32), "-expected-version", "1", "-reason", "replaced by follow-up", "-by", "planner", "-config", configPath})
	if callErr != nil {
		t.Fatalf("discard failed: %v: %s", callErr, discardOutput)
	}
	var discard steerline.TaskDiscardReceipt
	if err = json.Unmarshal(discardOutput, &discard); err != nil || discard.Status != "discarded" || discard.TaskID != "70" {
		t.Fatalf("unexpected discard receipt: %v: %+v", err, discard)
	}
	graphOutput, callErr := captureTaskCLI(t, []string{"graph", "-config", configPath, "-task-id", "70", "-direction", "upstream"})
	if callErr != nil {
		t.Fatalf("task graph failed: %v", callErr)
	}
	var graph steerline.TaskGraphPage
	if err = json.Unmarshal(graphOutput, &graph); err != nil || graph.Protocol != "native-task-graph/v1" || len(graph.Nodes) != 1 || graph.Nodes[0].TaskID != "70" || graph.Nodes[0].Status != "discarded" {
		t.Fatalf("unexpected task graph: %v / %+v", err, graph)
	}
	if _, callErr = captureTaskCLI(t, []string{"graph", "-config", configPath, "-limit", "1"}); callErr != nil {
		t.Fatalf("explicit graph paging failed: %v", callErr)
	}
	if _, callErr = captureTaskCLI(t, []string{"graph", "-config", configPath, "-cursor", "invalid"}); callErr == nil || !strings.Contains(callErr.Error(), "cursor_invalidated") {
		t.Fatalf("invalid graph cursor was accepted: %v", callErr)
	}
	if _, callErr = captureTaskCLI(t, []string{"graph", "-config", configPath, "-cursor", "", "-status", "open"}); callErr == nil {
		t.Fatal("graph cursor was mixed with status filtering")
	}
	exportPath := filepath.Join(t.TempDir(), "graph.json")
	if _, callErr = captureTaskCLI(t, []string{"export", "-config", configPath, "-format", "json", "-output", exportPath}); callErr != nil {
		t.Fatalf("task export failed: %v", callErr)
	}
	original, err := os.ReadFile(exportPath)
	if err != nil || !bytes.Contains(original, []byte(`"protocol": "native-task-graph/v1"`)) {
		t.Fatalf("export file is unreadable: %v: %s", err, original)
	}
	if _, callErr = captureTaskCLI(t, []string{"export", "-config", configPath, "-format", "json", "-output", exportPath}); callErr == nil || !strings.Contains(callErr.Error(), "output_exists") {
		t.Fatalf("export replaced an existing file without permission: %v", callErr)
	}
	if body, readErr := os.ReadFile(exportPath); readErr != nil || !bytes.Equal(body, original) {
		t.Fatalf("failed export changed the previous file: %v", readErr)
	}
	if _, callErr = captureTaskCLI(t, []string{"export", "-config", configPath, "-format", "json", "-output", filepath.Join(c.RunDir, "steerline.db-wal"), "-overwrite"}); callErr == nil || !strings.Contains(callErr.Error(), "cannot replace native task state") {
		t.Fatalf("export targeted a SQLite state file: %v", callErr)
	}
}

func captureTaskCLI(t *testing.T, args []string) ([]byte, error) {
	t.Helper()
	old := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = write
	callErr := runNativeTaskCommand(args)
	_ = write.Close()
	os.Stdout = old
	output, readErr := io.ReadAll(read)
	_ = read.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	return output, callErr
}
