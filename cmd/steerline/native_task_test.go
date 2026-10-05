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
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/erranliu/SteerLine/internal/console"
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	productclient "github.com/erranliu/SteerLine/internal/product/client"
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerlineapp"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestNativeTaskBatchCLIValidatesCreatesAndDiscardsFixedTaskIDs(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	profilePayload, _ := codexworker.EncodeProfileConfig(codexworker.ProfileConfig{Executable: "codex", WindowsSandbox: "elevated"})
	global, err := json.Marshal(map[string]any{
		"version":                   2,
		"execution_profiles":        []map[string]any{{"id": "local", "name": "Local Codex", "host": "local", "agent": "codex", "implementation_configs": []executionbackend.ImplementationConfigInput{{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 1, Payload: profilePayload}}}},
		"default_execution_profile": "local",
	})
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
	configJSON, _ := json.Marshal(map[string]any{"claimant": "cli-fixture", "repository": repo, "remote": "origin", "branch": "main", "task_authority": "native", "workflow_protocol": executionmodel.Version, "executable_types": []string{"task"}, "execution_policy": executionbackend.ExecutionPolicy{Filesystem: executionbackend.FilesystemPolicy{ReadScope: "host", WriteScope: "workspace", GitMetadataWrite: true}, Network: "allow", Escalation: "deny"}, "publication_policy": executionbackend.PublicationPolicy{Publisher: executionbackend.PublisherWorker}, "pr": map[string]any{"kind": "command", "base_url": "https://github.com", "repository": "org/repo", "query": map[string]any{"executable": "gh"}}})
	if err = os.WriteFile(configPath, configJSON, 0600); err != nil {
		t.Fatal(err)
	}
	c, err := steerlineapp.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := steerlineapp.OpenNativeTaskProject(context.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	ref := func(name string) taskmodel.FileReference {
		blob, e := exec.Command("git", "-C", repo, "cat-file", "blob", commit+":"+name).Output()
		if e != nil {
			t.Fatal(e)
		}
		sum := sha256.Sum256(blob)
		return taskmodel.FileReference{Path: name, Commit: commit, SHA256: hex.EncodeToString(sum[:])}
	}
	contract := taskmodel.TaskContract{ContractID: "contract-example", Repository: remoteURL, Remote: "origin", Branch: "main", SpecRef: ref("docs/spec/example.spec.md"), PlanRef: ref("docs/spec/example.plan.md"), CommonRefs: []taskmodel.FileReference{ref("docs/spec/native-task-constraints.md")}}
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
	graphOutput, callErr := captureTaskCLI(t, []string{"graph", "-config", configPath, "-task-id", "70", "-direction", "upstream"})
	if callErr != nil {
		t.Fatalf("task graph failed: %v", callErr)
	}
	var graph productapi.GraphPage
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
	t.Run("CM71 control keeps original request and client close preserves service", func(t *testing.T) {
		// Construct the real service identity/checkpoint without dispatching a
		// worker. CLI publication and receipt queries use production file I/O.
		service, err := steerlineapp.NewService(c)
		if err != nil {
			t.Fatal(err)
		}
		state := service.State
		state.Service.Mode, state.Service.Ready = "scheduler", true
		at := time.Now().UTC()
		checkpoint := reconstruction.Checkpoint(state)
		checkpoint.SchemaVersion, checkpoint.UpdatedAt, checkpoint.HeartbeatAt = state.SchemaVersion, at, at
		if err = service.Close(); err != nil {
			t.Fatal(err)
		}
		writer, err := sqlite.OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
		if err != nil {
			t.Fatal(err)
		}
		_, err = writer.SaveCheckpoint(context.Background(), checkpoint)
		if closeErr := writer.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			t.Fatal(err)
		}
		capture := func(args []string) ([]byte, error) {
			oldArgs := os.Args
			os.Args = append([]string{"steerline"}, args...)
			defer func() { os.Args = oldArgs }()
			oldOut := os.Stdout
			read, write, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			os.Stdout = write
			err = run()
			_ = write.Close()
			os.Stdout = oldOut
			output, readErr := io.ReadAll(read)
			_ = read.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
			return output, err
		}
		output, err := capture([]string{"control", "-config", configPath, "-action", "set_dispatch_mode", "-mode", "manual", "-by", "operator"})
		if err != nil {
			t.Fatalf("real CLI control: %s / %v", output, err)
		}
		fields := strings.Fields(string(output))
		if len(fields) != 2 || fields[0] != "request_id" {
			t.Fatalf("request output changed: %s", output)
		}
		id := fields[1]
		client, err := steerlineapp.OpenClient(context.Background(), configPath)
		if err != nil {
			t.Fatal(err)
		}
		req, err := client.Request(context.Background(), id)
		if err != nil || req.InstanceID != state.Service.InstanceID || req.RepositoryIdentity != c.Identity() || req.ExpectedControlRevision != state.Control.Revision {
			t.Fatalf("fixed request target: %+v / %v", req, err)
		}
		input := productapi.ControlInput{Action: productapi.SetDispatchMode, Payload: productapi.ControlPayload{Mode: "manual"}, RequestID: id, RequestedBy: "operator", InstanceID: req.InstanceID, ExpectedRevision: &req.ExpectedControlRevision}
		if _, err = client.SubmitControl(context.Background(), input); err != nil {
			t.Fatalf("immutable request replay: %v", err)
		}
		input.Payload.Mode = "auto"
		if _, err = client.SubmitControl(context.Background(), input); err == nil {
			t.Fatal("same ID different content accepted")
		}
		raw, err := os.ReadFile(filepath.Join(c.RunDir, "control", "requests", id, "request.json"))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(raw)
		receipt := productapi.ControlReceipt{Version: 1, RequestID: id, RequestDigest: hex.EncodeToString(digest[:]), InstanceID: req.InstanceID, Action: req.Action, Status: "indeterminate", Code: "outcome_unknown"}
		receiptBody, _ := json.Marshal(receipt)
		receiptDir := filepath.Join(c.RunDir, "control", "receipts")
		if err = os.MkdirAll(receiptDir, 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(receiptDir, id+".json"), receiptBody, 0600); err != nil {
			t.Fatal(err)
		}
		output, err = capture([]string{"control", "-config", configPath, "-request-id", id})
		if err != nil {
			t.Fatal(err)
		}
		var queried productapi.ControlReceipt
		if err = json.Unmarshal(output, &queried); err != nil || queried.RequestID != id || queried.Status != "indeterminate" {
			t.Fatalf("unknown CLI receipt became success or changed ID: %s / %v", output, err)
		}

		t.Run("CM72 actual console shares product adapter and original receipt", func(t *testing.T) {
			refs, options, err := steerlineapp.PrepareConsole(context.Background(), configPath, "")
			if err != nil || len(refs) != 1 || len(options.Clients) != 1 {
				t.Fatalf("console construction: %+v / %v", refs, err)
			}
			tuiClient := options.Clients[refs[0].Key]
			if _, ok := tuiClient.(*productclient.Client); !ok {
				t.Fatal("TUI uses another client implementation")
			}
			cliProfile := client.Project().Presentation
			profiles := refs[0].Presentation.Profiles
			if len(profiles) == 0 || len(profiles) != len(cliProfile.Profiles) || profiles[0].Digest == "" || profiles[0].Digest != cliProfile.Profiles[0].Digest || refs[0].Presentation.ConfigDigest != c.LoadedConfigDigest() {
				t.Fatal("CLI and TUI adopted different accepted profiles")
			}
			if result, err := tuiClient.Receipt(context.Background(), id); err != nil || result.Status != "indeterminate" || result.RequestID != id {
				t.Fatalf("TUI changed original CLI receipt: %+v / %v", result, err)
			}
			if snapshot := tuiClient.ReadProject(context.Background(), "70", productapi.ProjectSnapshot{}); snapshot.Connection.State != productapi.ConnectionConnected || snapshot.HistoryError != "" {
				t.Fatalf("actual console failed reading project: %+v", snapshot)
			}

			detached := tuiClient.Project()
			detached.Presentation.Profiles[0].Digest = "consumer-mutated"
			detached.Presentation.Profiles[0].ImplementationDisplay[0] = "consumer-mutated"
			if actual := tuiClient.Project(); actual.Presentation.Profiles[0].Digest != profiles[0].Digest || actual.Presentation.Profiles[0].ImplementationDisplay[0] == "consumer-mutated" {
				t.Fatal("view mutated accepted profile")
			}
			if _, err := tuiClient.InspectRecovery(context.Background(), "missing-run"); err == nil || !strings.Contains(err.Error(), "not found") {
				t.Fatalf("actual recovery read port missing: %v", err)
			}
			model, err := console.NewModel(context.Background(), refs, options)
			if err != nil {
				t.Fatal(err)
			}
			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
			if cmd == nil {
				t.Fatal("TUI q did not exit")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatal("TUI q invoked a service action")
			}
			if err := updated.(console.Model).Close(); err != nil {
				t.Fatal(err)
			}
			if connection := steerline.ConnectService(context.Background(), c); connection.State != steerline.ConnectionConnected {
				t.Fatalf("TUI exit stopped service: %+v", connection)
			}
			if _, err := tuiClient.Request(context.Background(), id); err == nil {
				t.Fatal("closed UI client retained resources")
			}
		})
		if err = client.Close(); err != nil {
			t.Fatal(err)
		}
		if connection := steerline.ConnectService(context.Background(), c); connection.State != steerline.ConnectionConnected {
			t.Fatalf("client close changed service identity/lifetime: %+v", connection)
		}
		unchanged, err := os.ReadFile(filepath.Join(c.RunDir, "control", "requests", id, "request.json"))
		if err != nil || !bytes.Equal(raw, unchanged) {
			t.Fatal("replay/close replaced original request")
		}
		// Even a self-consistent receipt copied into this project's directory
		// must not become a current-project result when its request is foreign.
		req.RepositoryIdentity = "foreign-project"
		foreign, _ := json.Marshal(req)
		foreignDigest := sha256.Sum256(foreign)
		receipt.RequestDigest = hex.EncodeToString(foreignDigest[:])
		receiptBody, _ = json.Marshal(receipt)
		if err = os.WriteFile(filepath.Join(c.RunDir, "control", "requests", id, "request.json"), foreign, 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(receiptDir, id+".json"), receiptBody, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = capture([]string{"control", "-config", configPath, "-request-id", id}); err == nil || !strings.Contains(err.Error(), "project identity") {
			t.Fatalf("foreign receipt was shown under current project: %v", err)
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
