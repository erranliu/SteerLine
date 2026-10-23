package main

import (
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
	"time"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestNativeTaskBatchCLIValidatesCreatesAndDiscardsFixedTaskIDs(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-read-only")
	if err := os.MkdirAll(steerlineapp.EtcDir(), 0700); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	rawExe, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	exeSum := sha256.Sum256(rawExe)
	global, err := json.Marshal(map[string]any{"version": 3, "runner": map[string]string{"executable": exe, "version": "unit-read-only", "sha256": hex.EncodeToString(exeSum[:])}, "execution_profiles": []any{map[string]any{"id": "local", "name": "Local Codex", "execution": map[string]any{"adapter": "codex-app-server", "target": map[string]string{"kind": "local"}, "settings": map[string]string{"executable": "codex.exe"}}, "workflow": map[string]any{"id": "development", "version": 1}}}, "default_execution_profile": "local"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(steerlineapp.GlobalExecutionConfigPath(), global, 0600); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(t.TempDir(), "repo")
	remoteURL := "https://github.com/org/repo.git"
	for _, args := range [][]string{{"init", "--quiet", "-b", "main", repo}, {"-C", repo, "remote", "add", "origin", remoteURL}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	files := map[string]string{"SteerlineFlow/spec/example.spec.md": "# Example Spec\n\n## Acceptance\n\nPass.\n", "SteerlineFlow/plans/example.plan.md": "# Example Plan\n", "SteerlineFlow/spec/native-task-constraints.md": "# Shared Constraints\n"}
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
	configPath := filepath.Join(steerlineapp.EtcDir(), "native.json")
	configJSON, _ := json.Marshal(map[string]any{"claimant": "cli-fixture", "repository": repo, "remote": "origin", "branch": "main", "task_authority": "native", "workflow_protocol": executionmodel.Version, "executable_types": []string{"task"}, "publication_policy": executionbackend.PublicationPolicy{Publisher: executionbackend.PublisherWorker}, "pr": map[string]any{"kind": "command", "base_url": "https://github.com", "repository": "org/repo", "query": map[string]any{"executable": "git"}}})
	if err = os.WriteFile(configPath, configJSON, 0600); err != nil {
		t.Fatal(err)
	}
	c, err := steerlineapp.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.ResolveIdentity(context.Background()); err != nil {
		t.Fatal(err)
	}
	startCLIHost(t, configPath)
	ref := func(name string) taskmodel.FileReference {
		blob, e := exec.Command("git", "-C", repo, "cat-file", "blob", commit+":"+name).Output()
		if e != nil {
			t.Fatal(e)
		}
		sum := sha256.Sum256(blob)
		return taskmodel.FileReference{Path: name, Commit: commit, SHA256: hex.EncodeToString(sum[:])}
	}
	contract := taskmodel.TaskContract{ContractID: "contract-example", Repository: remoteURL, Remote: "origin", Branch: "main", SpecRef: ref("SteerlineFlow/spec/example.spec.md"), PlanRef: ref("SteerlineFlow/plans/example.plan.md"), CommonRefs: []taskmodel.FileReference{ref("SteerlineFlow/spec/native-task-constraints.md")}}
	contract.AcceptanceRefs = []taskmodel.FileReference{contract.SpecRef}
	contract.Digest, err = taskmodel.ContractDigest(contract)
	if err != nil {
		t.Fatal(err)
	}
	contract2 := contract
	contract2.ContractID = "contract-second"
	contract2.Digest, err = taskmodel.ContractDigest(contract2)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(taskmodel.TaskBatchInput{SchemaVersion: "task-batch/v1", ProjectID: c.Identity(), Tasks: []taskmodel.TaskBatchTask{{LocalKey: "fixed-task", TaskID: 70, Title: "Fixed planned task", Type: "task", Contract: contract}, {LocalKey: "second-task", TaskID: 71, Title: "Second task", Type: "task", Contract: contract2}}, Dependencies: []taskmodel.TaskBatchEdge{}})
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
	var validation taskmodel.TaskBatchValidation
	if err = json.Unmarshal(validationOutput, &validation); err != nil || !validation.Valid || validation.AuthorityVersion == 0 {
		t.Fatalf("unexpected validation output: %v: %s", err, validationOutput)
	}
	requestID := strings.Repeat("1", 32)
	createOutput, callErr := captureTaskCLI(t, []string{"create", "-file", batchPath, "-request-id", requestID, "-expected-authority-version", fmt.Sprint(validation.AuthorityVersion), "-by", "planner", "-config", configPath})
	if callErr != nil {
		t.Fatalf("create failed: %v: %s", callErr, createOutput)
	}
	var receipt taskmodel.TaskBatchReceipt
	if err = json.Unmarshal(createOutput, &receipt); err != nil || receipt.TaskIDs["fixed-task"] != "70" || !strings.Contains(receipt.DispatchNote, "does not start work") {
		t.Fatalf("unexpected create receipt: %v: %+v", err, receipt)
	}
	addOutput, callErr := captureTaskCLI(t, []string{"add-predecessors", "-task-id", "71", "-predecessors", "70", "-request-id", strings.Repeat("3", 32), "-expected-version", "1", "-expected-graph-revision", "1", "-reason", "second task now requires first", "-by", "planner", "-config", configPath})
	if callErr != nil {
		t.Fatalf("add-predecessors failed: %v: %s", callErr, addOutput)
	}
	var added productapi.TaskPredecessorAddReceipt
	if err = json.Unmarshal(addOutput, &added); err != nil || added.TaskVersion != 2 || added.GraphRevision != 2 || added.TaskID != "71" {
		t.Fatalf("unexpected predecessor receipt: %v: %+v", err, added)
	}
	discardOutput, callErr := captureTaskCLI(t, []string{"discard", "-task-id", "70", "-request-id", strings.Repeat("2", 32), "-expected-version", "1", "-reason", "replaced by follow-up", "-by", "planner", "-config", configPath})
	if callErr != nil {
		t.Fatalf("discard failed: %v: %s", callErr, discardOutput)
	}
	var discard taskmodel.TaskDiscardReceipt
	if err = json.Unmarshal(discardOutput, &discard); err != nil || discard.Status != "discarded" || discard.TaskID != "70" {
		t.Fatalf("unexpected discard receipt: %v: %+v", err, discard)
	}
	graphOutput, callErr := captureTaskCLI(t, []string{"graph", "-config", configPath, "-limit", "1"})
	if callErr != nil {
		t.Fatalf("task graph failed: %v", callErr)
	}
	var graph productapi.GraphPage
	if err = json.Unmarshal(graphOutput, &graph); err != nil || graph.Protocol != "native-task-graph/v2" || len(graph.Nodes) != 1 || graph.Nodes[0].TaskID != "70" || graph.Nodes[0].Status != "discarded" {
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
	t.Run("control original receipt and client close preserve runtime", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		client, err := steerlineapp.OpenApplicationProject(ctx, "", configPath)
		if err != nil {
			t.Fatal(err)
		}
		before := client.Identity()
		request, err := client.SubmitControl(ctx, productapi.ControlInput{RequestID: strings.Repeat("9", 32), Action: productapi.SetDispatchMode, Payload: productapi.ControlPayload{Mode: "manual"}})
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := client.WaitControl(ctx, request.RequestID, 100*time.Millisecond)
		if err != nil || receipt.RequestID != request.RequestID || receipt.Status != "applied" {
			t.Fatal(receipt, err)
		}
		client.Close()
		reopened, err := steerlineapp.OpenApplicationProject(ctx, "", configPath)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		if reopened.Identity().InstanceID != before.InstanceID {
			t.Fatal("client close changed runtime")
		}
		original, err := reopened.Receipt(ctx, request.RequestID)
		if err != nil || original.RequestID != request.RequestID || original.InstanceID != before.InstanceID {
			t.Fatal("original receipt changed", original, err)
		}
	})
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
