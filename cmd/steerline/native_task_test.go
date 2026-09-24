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
	if page.Protocol != "native-tasks/v1" || page.ProjectID == "" || page.SampledAt.IsZero() || len(page.Tasks) != 0 || page.Capabilities.DependencyReadiness != "provided" {
		t.Fatalf("unexpected native task list: %+v", page)
	}
	if err = runNativeTaskCommand([]string{"show", "-task-id", "01", "-config", configPath}); err == nil || !strings.Contains(err.Error(), "contract_mismatch") {
		t.Fatalf("noncanonical task ID was accepted: %v", err)
	}
}

func TestNativeTaskBatchCLIValidatesCreatesAndDiscardsFixedTaskIDs(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
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
	configJSON, _ := json.Marshal(map[string]any{"repository": repo, "remote": "origin", "branch": "main", "task_authority": "native", "executable_types": []string{"task"}})
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
	payload, err := json.Marshal(steerline.TaskBatchInput{SchemaVersion: "task-batch/v1", ProjectID: c.Identity(), Tasks: []steerline.TaskBatchTask{{LocalKey: "fixed-task", TaskID: 70, Title: "Fixed planned task", Type: "task", Contract: contract}}, Dependencies: []steerline.TaskBatchEdge{}})
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
	discardOutput, callErr := captureTaskCLI(t, []string{"discard", "-task-id", "70", "-request-id", strings.Repeat("2", 32), "-expected-version", "1", "-reason", "replaced by follow-up", "-by", "planner", "-config", configPath})
	if callErr != nil {
		t.Fatalf("discard failed: %v: %s", callErr, discardOutput)
	}
	var discard steerline.TaskDiscardReceipt
	if err = json.Unmarshal(discardOutput, &discard); err != nil || discard.Status != "discarded" || discard.TaskID != "70" {
		t.Fatalf("unexpected discard receipt: %v: %+v", err, discard)
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
