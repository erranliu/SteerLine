package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	host "github.com/erranliu/SteerLine/internal/app/host"
	api "github.com/erranliu/SteerLine/internal/product/api"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

func TestRunnerPresetFreeze(t *testing.T) {
	t.Run("parameter_digest", func(t *testing.T) {
		p := RunnerProfile{ID: "plain", Name: "Original", Execution: runnerclient.ExecutionInput{Adapter: "codex-app-server", Target: runnerclient.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"model":"x","nested":{"b":2,"a":1}}`)}, Workflow: runnerclient.Workflow{ID: "development", Version: 1}}
		q := p
		q.ID = "renamed"
		q.Name = "New"
		q.Execution.Settings = json.RawMessage(`{ "nested":{"a":1,"b":2}, "model":"x" }`)
		if p.Digest() == "" || p.Digest() != q.Digest() {
			t.Fatal("name, ID or object ordering changed parameter digest")
		}
		q.Execution.Settings = json.RawMessage(`{"model":"x","nested":{"b":2,"a":1},"reasoning_effort":"medium"}`)
		if p.Digest() == q.Digest() {
			t.Fatal("explicit default was expanded away")
		}
		q = p
		require(t, json.Unmarshal([]byte(`{"id":"development","version":1,"prompt_overrides":{}}`), &q.Workflow))
		if p.Digest() == q.Digest() {
			t.Fatal("explicit empty workflow value was discarded")
		}
		for _, raw := range []string{`{"a":1,"a":2}`, `{"a":{"x":1,"x":2}}`, `{"items":[{"x":1,"x":2}]}`} {
			if _, err := runnercontract.CanonicalJSON([]byte(raw)); err == nil {
				t.Fatal("duplicate key accepted", raw)
			}
		}
		raw := []byte(`{"version":3,"version":3}`)
		var g RunnerExecutionConfig
		if err := decodeStrictJSON(raw, &g); err == nil {
			t.Fatal("original configuration lost duplicate keys before validation")
		}
	})
	t.Run("reject_replacement_in_live_runtime", func(t *testing.T) { testRuntimeReplacement(t, false) })
	t.Run("reject_mismatched_run_after_explicit_restart", func(t *testing.T) { testRuntimeReplacement(t, true, true) })
	t.Run("accept_replacement_after_explicit_restart", func(t *testing.T) { testRuntimeReplacement(t, true) })
	t.Run("reject_changed_build_without_compatible_takeover", func(t *testing.T) {
		c, _ := nativeTaskFixture(t)
		t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-fixture")
		require(t, os.MkdirAll(EtcDir(), 0700))
		require(t, os.WriteFile(filepath.Join(EtcDir(), "one.json"), projectConfigJSON(t, c), 0600))
		d, original := startPresetDirectory(t)
		a := original.runner.info.InstanceID
		require(t, d.Close())
		pinPath := filepath.Join(HomeDir(), "application-api", "runner-build.json")
		originalPin, err := os.ReadFile(pinPath)
		require(t, err)
		changed := c.RunnerExecution
		changed.Runner.Version = "unit-next-build"
		t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", changed.Runner.Version)
		require(t, os.WriteFile(GlobalExecutionConfigPath(), mustJSON(changed), 0600))
		// Current generations allow proven compatible takeover; express an
		// explicit public rejection rather than the retired unconditional gate.
		rejected := runnercontract.TakeoverReport{Contract: "steerrunner-takeover/v1", StateDir: original.runner.client.StateDir, CandidateSHA256: changed.Runner.SHA256, Compatible: false, Items: []runnercontract.TakeoverItem{}}
		require(t, os.WriteFile(filepath.Join(original.runner.client.StateDir, "idle-takeover-fixture.json"), mustJSON(rejected), 0600))
		next := NewApplicationDirectory(HomeDir())
		next.Start()
		defer next.Close()
		cat := switchCatalog(t, next, func(c contract.Catalog) bool { return len(c.Projects) == 1 && c.Projects[0].RuntimeState == "failed" })
		if cat.Projects[0].Online || !strings.Contains(cat.Projects[0].Diagnostic, "takeover") {
			t.Fatal("incompatible takeover was not rejected", cat)
		}
		currentPin, err := os.ReadFile(pinPath)
		require(t, err)
		if string(originalPin) != string(currentPin) {
			t.Fatal("rejected takeover rewrote the original build pin")
		}
		status, err := original.runner.client.ServiceStatus(context.Background())
		require(t, err)
		if status.Status != "not_running" || status.LastShutdown == nil || status.LastShutdown.InstanceID != a {
			t.Fatal("rejected build started/replaced service", status)
		}
	})
}

func startPresetDirectory(t *testing.T) (*ApplicationDirectory, *ProjectRuntime) {
	t.Helper()
	d := NewApplicationDirectory(HomeDir())
	d.Start()
	t.Cleanup(func() { _ = d.Close() })
	cat := switchCatalog(t, d, func(c contract.Catalog) bool {
		return len(c.Projects) == 1 && (c.Projects[0].Online || c.Projects[0].RuntimeState == "failed")
	})
	if !cat.Projects[0].Online {
		t.Fatalf("runtime failed: %+v", cat)
	}
	return d, d.entries[0].runtime
}

func testRuntimeReplacement(t *testing.T, restart bool, invalid ...bool) {
	c, _ := nativeTaskFixture(t)
	if restart {
		require(t, os.WriteFile(filepath.Join(c.Repository, "spec.md"), []byte("# Spec\n\n## Acceptance\n\nFrozen task.\n"), 0600))
		require(t, os.WriteFile(filepath.Join(c.Repository, "plan.md"), []byte("# Plan\n\nFrozen implementation plan.\n"), 0600))
		fixtureGit(t, c.Repository, "add", "spec.md", "plan.md")
		fixtureGit(t, c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "frozen task references")
	}
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-fixture")
	require(t, os.MkdirAll(EtcDir(), 0700))
	require(t, os.WriteFile(filepath.Join(EtcDir(), "one.json"), projectConfigJSON(t, c), 0600))
	d, old := startPresetDirectory(t)
	var original runnerstate.Candidate
	if restart {
		original = seedStoppedPublicRun(t, old, c)
	}
	a := old.runner.info.InstanceID
	path := filepath.Join(old.runner.client.StateDir, "idle-fixture.json")
	raw, err := os.ReadFile(path)
	require(t, err)
	var state map[string]any
	require(t, json.Unmarshal(raw, &state))
	b := "replacement-" + a
	t.Logf("original runtime=%p instance A=%s; same build/state-dir replacement B=%s", old, a, b)
	state["instance_id"] = b
	require(t, os.WriteFile(path, mustJSON(state), 0600))
	// All commands use the bound public process client; no business invocation
	// may reach the idle CLI after replacement, including observation/control.
	for _, command := range []string{"submit", "reply", "resume", "environment-check start", "inspect"} {
		request := &runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "new-" + command, Input: json.RawMessage(`{}`)}
		if command == "inspect" {
			request = nil
		}
		_, err = old.runner.client.Call(context.Background(), command, nil, request)
		if err == nil || !strings.Contains(err.Error(), "restart SteerLine explicitly") {
			t.Fatalf("%s was not fenced: %v", command, err)
		}
	}
	input := "{}"
	for i, action := range []api.ControlAction{api.StartTask, "runner_reply", "runner_resume"} {
		payload := api.ControlPayload{RunID: strings.Repeat("f", 32), ExpectedRunRevision: 1, RunnerInput: &input}
		if action == api.StartTask {
			payload = api.ControlPayload{TaskID: "999", ExpectedTaskVersion: 1, ExpectedGraphRevision: 1, ExecutionProfileID: "fixture", ExecutionProfileDigest: c.RunnerExecution.Profiles[0].Digest()}
		}
		req, err := old.product.SubmitControl(context.Background(), api.ControlInput{Action: action, RequestID: strings.Repeat(string(rune('1'+i)), 32), RequestedBy: "fixture", Payload: payload})
		require(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		receipt, err := old.product.WaitControl(ctx, req.RequestID, 10*time.Millisecond)
		cancel()
		require(t, err)
		if receipt.Status != "rejected" || !strings.Contains(receipt.Message, "restart SteerLine explicitly") {
			t.Fatalf("%s: %+v", action, receipt)
		}
	}
	if old.runner.client.InstanceID != a || old.runner.info.InstanceID != a {
		t.Fatal("old binding was mutated")
	}
	_, err = old.product.SubmitEnvironmentCheck(context.Background(), api.EnvironmentCheckCommand{RequestID: strings.Repeat("9", 32), Action: api.StartEnvironmentCheck, TargetID: c.Identity(), Payload: json.RawMessage(`{"profile_id":"fixture","access_mode":"sandbox"}`), Original: json.RawMessage(`{"request_id":"99999999999999999999999999999999"}`)})
	if err == nil || !strings.Contains(err.Error(), "restart SteerLine explicitly") {
		t.Fatal("project check was not fenced", err)
	}
	if err = old.runner.stop(context.Background()); err == nil {
		t.Fatal("A stop was accepted for B")
	}
	status, err := old.runner.client.ServiceStatus(context.Background())
	require(t, err)
	if status.InstanceID != b || status.Status != "ready" || old.runner.settle(context.Background()) == nil {
		t.Fatal("replacement falsely settled", status)
	}
	if _, err = os.Stat(filepath.Join(old.runner.client.StateDir, "business-dispatch.log")); !os.IsNotExist(err) {
		t.Fatal("business CLI dispatched after replacement", err)
	}
	if !restart {
		// Close reports the original resource as unresolved, while releasing locks.
		if err = d.Close(); err == nil {
			t.Fatal("replacement falsely settled original shutdown")
		}
		require(t, old.runner.client.StopService(context.Background(), b, "fixture-cleanup-B"))
		return
	}
	// Explicit host shutdown and reconstruction, never editing the old info.
	if err = d.Close(); err == nil {
		t.Fatal("original A unexpectedly settled")
	}
	// A same-build preset rename is admitted only by the reconstructed host;
	// history continues to display the candidate's original name.
	renamed := c.RunnerExecution
	renamed.Profiles[0].Name = "New preset name"
	require(t, os.WriteFile(GlobalExecutionConfigPath(), mustJSON(renamed), 0600))

	if len(invalid) > 0 && invalid[0] {
		observation := filepath.Join(old.runner.client.StateDir, "idle-run-fixture.json")
		raw, err := os.ReadFile(observation)
		require(t, err)
		var public map[string]any
		require(t, json.Unmarshal(raw, &public))
		public["run"].(map[string]any)["input_digest"] = strings.Repeat("d", 64)
		require(t, os.WriteFile(observation, mustJSON(public), 0600))
		next := NewApplicationDirectory(HomeDir())
		next.Start()
		t.Cleanup(func() { _ = next.Close() })
		catalog := switchCatalog(t, next, func(c contract.Catalog) bool { return len(c.Projects) == 1 && c.Projects[0].RuntimeState == "failed" })
		if catalog.Projects[0].Online || !strings.Contains(catalog.Projects[0].Diagnostic, "original Runner binding remains unverified") {
			t.Fatal("mismatched original Run was admitted", catalog)
		}
		if _, err := os.Stat(filepath.Join(old.runner.client.StateDir, "business-dispatch.log")); !os.IsNotExist(err) {
			t.Fatal("unverified restart dispatched business", err)
		}
		t.Log("replacement rejected before admission: public original input_digest differs; zero business dispatch")
		return
	}
	fresh, current := startPresetDirectory(t)
	if current == old || current.runner.info.InstanceID != b || current.runner.client.InstanceID != b {
		t.Fatal("explicit restart did not bind B")
	}
	bound, err := current.config.runnerClient(context.Background())
	require(t, err)
	checkRunner, err := newEnvironmentCheckPort(current.config, current.store).service.Open(context.Background(), current.config.RunnerExecution.Runner)
	require(t, err)
	if bound.InstanceID != b || checkRunner.(runnerclient.Client).InstanceID != b || old.config.runtimeRunner.InstanceID != a {
		t.Fatal("product/check/client bindings diverged after restart")
	}
	require(t, current.runner.client.ValidateInstance(context.Background()))
	if current.config.RunnerExecution.Profiles[0].Name != "New preset name" {
		t.Fatal("new runtime did not read new preset")
	}
	saved, err := current.store.Candidate(context.Background(), c.Identity(), original.ID)
	require(t, err)
	if saved.RunID != original.RunID || saved.InputDigest != original.InputDigest || saved.ProfileName != original.ProfileName || string(mustJSON(saved.Submit)) != string(mustJSON(original.Submit)) || string(mustJSON(saved.Actions)) != string(mustJSON(original.Actions)) {
		t.Fatal("explicit restart changed frozen Run/input", saved, original)
	}
	t.Logf("rebuilt runtime=%p instance=%s preserved Run=%s input_digest=%s submit=%s frozen_name=%q; zero new business dispatch", current, b, saved.RunID, saved.InputDigest, saved.Submit.RequestID, saved.ProfileName)
	if _, err = os.Stat(filepath.Join(current.runner.client.StateDir, "business-dispatch.log")); !os.IsNotExist(err) {
		t.Fatal("restart automatically sent business input", err)
	}
	if err = old.runner.stop(context.Background()); err == nil {
		t.Fatal("replayed A stop accepted")
	}
	status, err = current.runner.client.ServiceStatus(context.Background())
	require(t, err)
	if status.Status != "ready" || status.InstanceID != b {
		t.Fatal("replay stopped B", status)
	}
	require(t, fresh.Close())
	require(t, current.runner.ResourceSettlementError())
	status, err = current.runner.client.ServiceStatus(context.Background())
	require(t, err)
	if status.LastShutdown == nil || status.LastShutdown.InstanceID != b || status.LastShutdown.RequestID != "sl-service-stop-"+b {
		t.Fatal("new shutdown lost B proof", status)
	}
	t.Logf("A stop replay refused; B final proof instance=%s request=%s settled=%v", status.LastShutdown.InstanceID, status.LastShutdown.RequestID, status.LastShutdown.ResourcesSettled)
}

func seedStoppedPublicRun(t *testing.T, r *ProjectRuntime, c Config) runnerstate.Candidate {
	t.Helper()
	ctx := context.Background()
	base := fixtureGit(t, c.Repository, "rev-parse", "HEAD")
	ref := func(name string) taskmodel.FileReference {
		raw, err := os.ReadFile(filepath.Join(c.Repository, name))
		require(t, err)
		sum := sha256.Sum256(raw)
		return taskmodel.FileReference{Path: name, Commit: base, SHA256: hex.EncodeToString(sum[:])}
	}
	spec, plan := ref("spec.md"), ref("plan.md")
	contract := taskmodel.TaskContract{ContractID: "runtime-binding", Repository: "https://github.com/owner/repo.git", Remote: "origin", Branch: "main", SpecRef: spec, PlanRef: plan, AcceptanceRefs: []taskmodel.FileReference{spec}, CommonRefs: []taskmodel.FileReference{}}
	var err error
	contract.Digest, err = taskmodel.ContractDigest(contract)
	require(t, err)
	graph, err := r.store.TaskGraphSnapshot(ctx, c.Identity())
	require(t, err)
	batch := taskmodel.TaskBatchInput{SchemaVersion: taskmodel.BatchSchemaVersion, ProjectID: c.Identity(), Tasks: []taskmodel.TaskBatchTask{{LocalKey: "a", TaskID: 1, Type: "task", Title: "Frozen task", Contract: contract}}, Dependencies: []taskmodel.TaskBatchEdge{}}
	_, err = r.product.CreateTaskBatch(ctx, api.CreateInput{Payload: mustJSON(batch), RequestID: strings.Repeat("8", 32), RequestedBy: "fixture", ExpectedAuthorityVersion: graph.Authority.AuthorityVersion})
	require(t, err)
	_, err = r.product.SubmitControl(ctx, api.ControlInput{Action: api.ResumeDispatch, RequestID: strings.Repeat("7", 32), RequestedBy: "fixture"})
	require(t, err)
	wait, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	// The fixture already owns the frozen host binding. Use its verified
	// receipt reader without repeating Git/config discovery on every poll.
	_, err = host.WaitControlReceipt(wait, r.hostConfig, strings.Repeat("7", 32), 10*time.Millisecond)
	if err != nil {
		t.Fatalf("fixture resume receipt: %v", err)
	}
	graph, err = r.store.TaskGraphSnapshot(ctx, c.Identity())
	require(t, err)
	public := runnercontract.Run{RunID: strings.Repeat("b", 32), Revision: 1, InputDigest: strings.Repeat("c", 64), ResourceDigest: r.runner.info.ResourceDigest, Workflow: c.RunnerExecution.Profiles[0].Workflow, Status: "stopped", StopStatus: "stopped"}
	requestID := strings.Repeat("a", 32)
	require(t, os.WriteFile(filepath.Join(r.runner.client.StateDir, "idle-run-fixture.json"), mustJSON(map[string]any{"run": public, "command": map[string]any{"request_id": "sl-submit-" + requestID, "command_id": "frozen-submit-command", "command": "submit", "run_id": public.RunID, "receipt_status": "accepted", "action_status": "completed"}}), 0600))
	// Reserve through the actual caller service; the idle peer supplies only an
	// existing stopped Run observation, never a model or business execution.
	service := runnerTaskService(r.config, r.store, r.runner.client)
	service.ResolveBase = func(context.Context, taskmodel.FixedWork, bool) (string, error) { return base, nil }
	candidate, err := service.Start(ctx, taskrunner.Start{RequestID: requestID, TaskID: "1", TaskVersion: 1, GraphRevision: graph.Authority.GraphRevision, Explicit: true})
	require(t, err)
	require(t, os.WriteFile(filepath.Join(r.runner.client.StateDir, "idle-run-fixture.json"), mustJSON(map[string]any{"run": public, "command": map[string]any{"request_id": candidate.Submit.RequestID, "command_id": "frozen-submit-command", "command": "submit", "run_id": public.RunID, "receipt_status": "accepted", "action_status": "completed"}}), 0600))
	// Let the running actor's own bridge observe the static public fact. A
	// second, test-local bridge would race that writer and fail its candidate
	// CAS. Polling committed facts neither retries a command nor weakens CAS.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		candidate, err = r.store.Candidate(ctx, c.Identity(), candidate.ID)
		require(t, err)
		if candidate.RunID == public.RunID && candidate.InputDigest == public.InputDigest && candidate.Cursor == "idle-end" && candidate.Projection != nil && candidate.Projection.Status == "stopped" {
			return candidate
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("actor did not commit the public stopped Run observation", candidate)
	return candidate
}
