//go:build runneracceptance

package runnerclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/steeragent/maintenance"
)

// One caller/public-EXE boundary test. The native protocol fixture is supplied
// by Runner; this does not duplicate its collector matrix or prove a real PR.
func TestProjectEnvironmentExecutionPublishedResultRoundTrip(t *testing.T) {
	testPublishedResultFile(t, "")
}

func TestNative101PublishedArrangement(t *testing.T) {
	for _, mode := range []string{"readable", "blocked"} {
		t.Run(mode, func(t *testing.T) { testPublishedResultFile(t, mode) })
	}
}

func TestNative101ActualAgentArrangement(t *testing.T) {
	if os.Getenv("STEERLINE_TASK101_CODEX") == "" || os.Getenv("STEERLINE_TASK101_REPOSITORY") == "" {
		t.Skip("explicit native Codex and task 101 repository required")
	}
	testPublishedResultFile(t, "native")
}

func testPublishedResultFile(t *testing.T, arrangementMode string) {
	t.Helper()
	const arrangement101 = `{"tests":["N101-CONTENT","N101-SCOPE","demo.progress.roundtrip","demo.progress.retry","demo.progress.cleanup"]}`
	expectedDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(arrangement101)))
	exe, native := os.Getenv("STEERRUNNER_EXE"), os.Getenv("STEERRUNNER_NATIVE_FIXTURE")
	if exe == "" || native == "" {
		t.Fatal("explicit pinned Runner and its native protocol fixture required")
	}
	root, err := os.MkdirTemp("", "sl-result-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("preserved failed caller fixture: %s", root)
			return
		}
		if filepath.Dir(root) != filepath.Clean(os.TempDir()) || !strings.HasPrefix(filepath.Base(root), "sl-result-") {
			t.Error("refuse cleanup outside owned fixture")
			return
		}
		deadline := time.Now().Add(8 * time.Second)
		for {
			err := os.RemoveAll(root)
			if err == nil {
				return
			}
			if time.Now().After(deadline) {
				t.Error(err)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	})
	check := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	source, bare, state := filepath.Join(root, "source"), filepath.Join(root, "remote.git"), filepath.Join(root, "native")
	check(os.MkdirAll(source, 0700))
	check(os.MkdirAll(state, 0700))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(root, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	if arrangementMode != "native" {
		t.Setenv("CODEX_HOME", state)
	}
	t.Setenv("STEERRUNNER_FIXTURE_STATE", state)
	// This contract exercises collection during a held native Turn. It does
	// not ask the protocol fixture to recognize or implement business prompts.
	t.Setenv("STEERRUNNER_CONTENT_FIXTURE", "1")
	if arrangementMode != "" {
		t.Setenv("STEERRUNNER_EXPECT_ARRANGEMENT_SHA256", expectedDigest)
	}
	check(os.WriteFile(filepath.Join(state, "content-final"), []byte("controlled held Turn"), 0600))
	provider := filepath.Join(root, "provider.json")
	t.Setenv("STEERRUNNER_FIXTURE_PROVIDER", provider)
	check(os.WriteFile(filepath.Join(state, "hold-turn"), []byte("hold"), 0600))
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		raw, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, raw, err)
		}
		return strings.TrimSpace(string(raw))
	}
	git(root, "init", "--bare", bare)
	git(source, "init", "-b", "main")
	spec, plan := "spec.md", "plan.md"
	if task101Source := os.Getenv("STEERLINE_TASK101_REPOSITORY"); (arrangementMode == "readable" || arrangementMode == "native") && task101Source != "" {
		const fixed = "9d81d9c2652177faf6354d4e9f9f3aec79535d11"
		git(source, "fetch", "--no-tags", "--", task101Source, fixed)
		git(source, "-c", "core.hooksPath=/dev/null", "checkout", "-B", "main", fixed)
		spec, plan = "SteerlineFlow/spec/native-101.spec.md", "SteerlineFlow/plans/native-101.plan.md"
		if got := git(source, "ls-tree", fixed, "--", "SteerlineFlow/plans/native-101.tests.json"); got != "" {
			t.Fatal("task 101 fixed commit unexpectedly contains a sidecar")
		}
	} else {
		check(os.WriteFile(filepath.Join(source, spec), []byte("Controlled caller result contract"), 0600))
		check(os.WriteFile(filepath.Join(source, plan), []byte("Hold native fixture; caller stops original Run"), 0600))
		if arrangementMode == "blocked" {
			check(os.Mkdir(filepath.Join(source, ".steerrunner"), 0700))
			check(os.WriteFile(filepath.Join(source, ".steerrunner", "result"), []byte("blocks result directory creation"), 0600))
		}
		git(source, "add", ".")
		git(source, "-c", "user.name=Caller Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "fixture")
	}
	commit := git(source, "rev-parse", "HEAD")
	git(root, "config", "--global", "url."+filepath.ToSlash(bare)+".insteadOf", "https://controlled.invalid/owner/repo.git")
	git(source, "remote", "add", "origin", "https://controlled.invalid/owner/repo.git")
	git(source, "push", "origin", "main")
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	selection := Selection{Executable: exe, Version: os.Getenv("STEERRUNNER_VERSION"), SHA256: os.Getenv("STEERRUNNER_SHA256")}
	c, version, err := selection.Open(ctx, filepath.Join(root, "runner"), 30*time.Second)
	check(err)
	info, err := c.StartService(ctx, version)
	check(err)
	var runID string
	var stop *Request
	var stopRevision uint64
	// Cleanup only addresses the original Run/request and original service.
	stopRun := func() error {
		v, err := c.Inspect(context.Background(), "run-id", runID)
		if err != nil || v.Run.StopStatus == "stopped" {
			return err
		}
		if stop == nil {
			stopRevision = v.Run.Revision
			body, _ := json.Marshal(map[string]uint64{"expected_revision": stopRevision})
			stop = &Request{Protocol: Protocol, RequestID: "caller-result-stop", Input: body}
			_, err = c.Call(context.Background(), "stop", []string{"--run-id", runID}, stop)
		}
		return err
	}
	observeStoppedArrangement := func() {
		t.Helper()
		cursor := ""
		for ctx.Err() == nil {
			observation, err := c.Observe(ctx, runID, cursor)
			check(err)
			cursor = observation.EventPage.NextCursor
			if observation.Run.StopStatus == "stopped" {
				if stop == nil || observation.Run.Revision <= stopRevision {
					t.Fatal("stop observation did not advance the pre-control revision", stopRevision, observation.Run.Revision)
				}
				t.Logf("101 stop observation: Run=%s before=%d after=%d", runID, stopRevision, observation.Run.Revision)
				return
			}
			time.Sleep(250 * time.Millisecond)
		}
		t.Fatal("native arrangement read passed but observe did not confirm stop")
	}
	t.Cleanup(func() {
		if runID != "" {
			if err := stopRun(); err != nil {
				t.Error(err)
			}
		}
		if err := c.StopService(context.Background(), info.InstanceID, "caller-result-service-stop"); err != nil {
			t.Error(err)
		} else if err := c.VerifyServiceSettlement(context.Background(), info.InstanceID, "caller-result-service-stop"); err != nil {
			t.Error(err)
		}
	})
	c.InstanceID = info.InstanceID
	cat, err := c.Catalog(ctx, version)
	check(err)
	check(cat.ValidateProgress())
	check(cat.ValidateResult())
	frozenSchema, err := os.ReadFile("testdata/result-file.schema.json")
	check(err)
	var frozen, published any
	check(json.Unmarshal(frozenSchema, &frozen))
	check(json.Unmarshal(cat.Capabilities.ResultFileSchema, &published))
	if !reflect.DeepEqual(frozen, published) {
		t.Fatal("published result schema and frozen caller fixture differ")
	}
	c = c.WithCatalog(cat)
	engine := native
	if arrangementMode == "native" {
		engine = os.Getenv("STEERLINE_TASK101_CODEX")
	}
	settings, _ := json.Marshal(map[string]string{"executable": engine, "access_mode": "full_access"})
	publication, _ := json.Marshal(map[string]any{"base_url": "https://controlled.invalid", "query": map[string]any{"executable": native, "args": []string{"--query", provider}}})
	input := runnercontract.RunInput{Title: "Caller result contract", Repository: runnercontract.Repository{URL: "https://controlled.invalid/owner/repo.git", LocalPath: source, Remote: "origin", BaseBranch: "main", BaseRevision: commit}, Documents: runnercontract.Documents{Spec: runnercontract.Document{Path: spec}, Plan: runnercontract.Document{Path: plan}}, Workflow: runnercontract.Workflow{ID: "development", Version: 2}, Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: settings}, Publication: runnercontract.Publication{Provider: "command", Repository: "owner/repo", BaseBranch: "main", Settings: publication}, ProgressFile: &runnercontract.ProgressFileInput{InitialContentBase64: base64.StdEncoding.EncodeToString([]byte(`{"test.one":false}`))}, ResultFile: &runnercontract.ResultFileInput{InitialContentBase64: base64.StdEncoding.EncodeToString([]byte("seed"))}}
	if arrangementMode != "" {
		input.Title = "Native 101"
		input.Workflow.ProjectPolicySlots = []string{}
		input.ResultFile.InitialContentBase64 = base64.StdEncoding.EncodeToString([]byte(arrangement101))
		if arrangementMode == "native" {
			input.Workflow.PromptOverrides = map[string]string{
				"implement.execute": "This is a read-only arrangement verification. Use a shell tool to compute the SHA-256 of the original file pointed to by STEERLINE_RESULT_FILE. Write only the hexadecimal digest to a new sibling file named arrangement-proof.txt. Leave the arrangement and progress files untouched. Do not implement, commit, or publish anything. Then report blocked so this diagnostic run stops before publication.",
			}
		}
		check(cat.ValidateResultTarget(input.Execution, input.Workflow))
		check(cat.ValidateConfiguration(input.Execution, input.Workflow))
	}
	body, _ := json.Marshal(input)
	receipt, err := c.Submit(ctx, Request{Protocol: Protocol, RequestID: "caller-result-submit", Input: body})
	check(err)
	runID = receipt.RunID
	var run *Run
	for ctx.Err() == nil {
		v, err := c.Inspect(ctx, "run-id", runID)
		check(err)
		run = v.Run
		if run.Execution.TurnID != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	check(ctx.Err())
	pending, err := c.ResultFile(ctx, runID)
	check(err)
	if pending.CollectionStatus != "pending" || pending.Version != 0 || pending.ContentBase64 != nil {
		t.Fatal("running result was exposed", pending)
	}
	path := filepath.Join(run.Execution.Workspace, ".steerrunner", "result", runID, "result.bin")
	if arrangementMode == "native" {
		proof := filepath.Join(filepath.Dir(path), "arrangement-proof.txt")
		for ctx.Err() == nil {
			data, err := os.ReadFile(proof)
			if err == nil && strings.EqualFold(strings.TrimSpace(string(data)), expectedDigest) {
				t.Logf("101 native Agent read: base=%s Run=%s SHA-256=%s", commit, runID, expectedDigest)
				for ctx.Err() == nil {
					view, err := c.Inspect(ctx, "run-id", runID)
					check(err)
					if view.Run.Execution.NativeOutcome != "" {
						break
					}
					time.Sleep(250 * time.Millisecond)
				}
				check(ctx.Err())
				check(stopRun())
				observeStoppedArrangement()
				return
			}
			time.Sleep(250 * time.Millisecond)
		}
		t.Fatal("native Agent did not produce the fixed arrangement digest")
	}
	if arrangementMode != "" {
		calls, err := os.ReadFile(filepath.Join(state, "calls.jsonl"))
		check(err)
		matched := false
		for _, line := range bytes.Split(calls, []byte{'\n'}) {
			var call struct {
				Method, Path, SHA256 string
				Readable, Blocked    bool
			}
			if json.Unmarshal(line, &call) != nil || call.Method != "arrangement/read" {
				continue
			}
			if filepath.Clean(call.Path) != filepath.Clean(path) || call.Blocked != (arrangementMode == "blocked") || call.Readable != (arrangementMode == "readable") || call.Readable && call.SHA256 != expectedDigest {
				t.Fatalf("unexpected Agent read trace: %+v", call)
			}
			matched = true
		}
		if !matched {
			t.Fatal("Runner did not dispatch a native input with an arrangement read")
		}
		progress, err := c.ProgressFile(ctx, runID)
		check(err)
		if progress.Version == 0 {
			t.Fatal("independent progress was not initialized")
		}
		t.Logf("101 %s: Runner=%s base=%s Run=%s arrangement SHA-256=%s", arrangementMode, version.SourceCommit, commit, runID, expectedDigest)
		check(stopRun())
		if arrangementMode == "readable" {
			observeStoppedArrangement()
		}
		return
	}
	final := []byte{0, 255, 'r', '\n'} // Result is opaque, including non-JSON bytes.
	check(os.WriteFile(path+".tmp", final, 0600))
	check(os.Rename(path+".tmp", path))
	progress, err := c.ProgressFile(ctx, runID)
	check(err)
	if progress.Version == 0 {
		t.Fatal("independent progress was not initialized")
	}
	check(stopRun())
	for ctx.Err() == nil {
		v, err := c.Inspect(ctx, "run-id", runID)
		check(err)
		if v.Run.StopStatus == "stopped" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	check(ctx.Err())
	sealed, err := c.ResultFile(ctx, runID)
	check(err)
	raw, err := ValidateResultFile(sealed, runID)
	check(err)
	if sealed.CollectionStatus != "closed" || sealed.Version != 1 || !bytes.Equal(raw, final) {
		t.Fatal("stopped final bytes differ", sealed)
	}
	check(os.WriteFile(path, []byte("late write"), 0600))
	again, err := c.ResultFile(ctx, runID)
	check(err)
	if !reflect.DeepEqual(sealed, again) {
		t.Fatal("sealed result refreshed")
	}
	t.Logf("public EXE result round trip: source=%s Run=%s version=%d SHA256=%s; controlled native fixture, no real PR", version.SourceCommit, runID, sealed.Version, sealed.SHA256)
	testMaintenancePublicReuse(t, ctx, c, cat, input, state)
}

// Exercise the caller's new fields against the fixed actual Runner. The native
// app-server peer is controlled; this proves no model judgement or real PR.
func testMaintenancePublicReuse(t *testing.T, ctx context.Context, c Client, cat *Catalog, base runnercontract.RunInput, nativeState string) {
	t.Helper()
	input := base
	executePrompt, promptErr := prompts.MaintenanceExecute([]byte(`{"action_id":"fixture","operation_id":"fixture","instruction":"controlled maintenance writer"}`))
	if promptErr != nil {
		t.Fatal(promptErr)
	}
	input.Workflow = runnercontract.Workflow{ID: "execute", Version: 1, PromptOverrides: map[string]string{"execute": executePrompt}}
	input.Publication = runnercontract.Publication{}
	input.ProgressFile, input.ResultFile = nil, nil
	read, err := c.ConfigureExecution(ctx, cat, runnercontract.ExecutionConfigurationRequest{Operation: "derive", Execution: &input.Execution, Workflow: runnercontract.Workflow{ID: "generate", Version: 1}, AccessMode: "read_only"})
	if err != nil || read.Execution == nil {
		t.Fatal(read, err)
	}
	if err = cat.ValidateMaintenance(input.Execution, *read.Execution); err != nil {
		t.Fatal(err)
	}
	runs := []string{}
	t.Cleanup(func() {
		for _, id := range runs {
			v, e := c.Inspect(context.Background(), "run-id", id)
			if e != nil {
				t.Error(e)
				continue
			}
			if v.Run.StopStatus != "stopped" {
				request := runnerbinding.ContentStop("cleanup-"+id, v.Run.Revision)
				if _, e = c.Call(context.Background(), "stop", []string{"--run-id", id}, request); e != nil {
					t.Error(e)
				}
			}
		}
	})
	wait := func(id string, done bool) *Run {
		t.Helper()
		for ctx.Err() == nil {
			v, e := c.Inspect(ctx, "run-id", id)
			if e != nil {
				t.Fatal(e)
			}
			if done && v.Run.StopStatus == "stopped" || !done && v.Run.Execution.TurnID != "" {
				return v.Run
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal(ctx.Err())
		return nil
	}
	latest, origin, identity, workspace, branch := "", "", "", "", ""
	sessions := map[string]bool{}
	for i := 0; i < 4; i++ {
		input.ReuseWorktreeFromRunID = latest

		body, _ := json.Marshal(input)
		receipt, e := c.Submit(ctx, Request{Protocol: Protocol, RequestID: fmt.Sprintf("maintenance-reuse-%d", i), Input: body})
		if e != nil {
			t.Fatal(e)
		}
		runs = append(runs, receipt.RunID)
		run := wait(receipt.RunID, false)
		if i == 0 {
			origin, identity, workspace, branch = run.WorkspaceOriginRunID, run.WorkspaceIdentity, run.Execution.Workspace, run.Execution.Branch
			if origin == "" || identity == "" || workspace == "" || branch == "" {
				t.Fatal("missing workspace incarnation", run)
			}
		}
		if run.ReuseWorktreeFromRunID != latest || run.WorkspaceOriginRunID != origin || run.WorkspaceIdentity != identity || run.Execution.Workspace != workspace || run.Execution.Branch != branch || sessions[run.Execution.SessionID] {
			t.Fatal("actual Runner reuse identity changed", run)
		}
		sessions[run.Execution.SessionID] = true
		dirty := filepath.Join(workspace, "maintenance-retained.txt")
		if i == 0 {
			if e = os.WriteFile(dirty, []byte("untracked retained fixture"), 0600); e != nil {
				t.Fatal(e)
			}
		} else {
			b, e := os.ReadFile(dirty)
			if e != nil || string(b) != "untracked retained fixture" {
				t.Fatal("worktree content lost", e)
			}
		}
		request := runnerbinding.ContentStop(fmt.Sprintf("maintenance-stop-%d", i), run.Revision)
		if _, e = c.Call(ctx, "stop", []string{"--run-id", run.RunID}, request); e != nil {
			t.Fatal(e)
		}
		wait(run.RunID, true)

		latest = run.RunID
	}
	// Maintenance decisions use the original project directory while the
	// writable Run keeps its worktree and untracked changes.
	decision := `{"kind":"finish","summary":"controlled read-only result"}`
	if err = os.WriteFile(filepath.Join(nativeState, "content-final"), []byte(decision), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(nativeState, "hold-turn")); err != nil {
		t.Fatal(err)
	}
	rules, contextBody := "controlled immutable file", "{\"mode\":\"first\"}"
	readPrompt, promptErr := prompts.MaintenanceDecision()
	if promptErr != nil {
		t.Fatal(promptErr)
	}
	g := runnercontract.GenerateRunInput{Title: "Maintenance readonly caller fixture", Workflow: runnercontract.Workflow{ID: "generate", Version: 1}, Execution: *read.Execution, Generate: runnercontract.GenerateInput{Directory: &runnercontract.DirectoryAccess{Path: input.Repository.LocalPath, Access: "read_only"}, Instruction: readPrompt, OutputSchema: maintenance.Schema("first"), Materials: []runnercontract.Document{{Kind: "file", Path: "maintenance/rules.md", Content: &rules}, {Kind: "file", Path: "maintenance/context.json", Content: &contextBody}}}}
	if err = cat.ValidateGenerate(g); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(g)
	receipt, err := c.Submit(ctx, Request{Protocol: Protocol, RequestID: "maintenance-directory", Input: body})
	if err != nil {
		t.Fatal(err)
	}
	runs = append(runs, receipt.RunID)
	run := wait(receipt.RunID, true)
	if !runnerbinding.SamePath(run.Execution.Workspace, input.Repository.LocalPath) || runnerbinding.SamePath(run.Execution.Workspace, workspace) || run.RepositorySource != nil || run.ContentResult == nil || run.ContentResult.Status != "succeeded" || run.Final == nil || !run.Final.Verified {
		t.Fatal("read-only directory result unavailable", run)
	}
	if err = VerifyFileMaterials(g.Generate.Materials, run.Materials); err != nil {
		t.Fatal(err)
	}
	if err = VerifyFileMaterials(g.Generate.Materials, run.ContentResult.FileMaterials); err != nil {
		t.Fatal(err)
	}
	g.Generate.Directory = nil
	g.Generate.OutputSchema = maintenance.Schema("select")
	contextBody = "{\"mode\":\"select\"}"
	if err = cat.ValidateGenerate(g); err != nil {
		t.Fatal(err)
	}
	body, _ = json.Marshal(g)
	receipt, err = c.Submit(ctx, Request{Protocol: Protocol, RequestID: "maintenance-context-only", Input: body})
	if err != nil {
		t.Fatal(err)
	}
	runs = append(runs, receipt.RunID)
	later := wait(receipt.RunID, true)
	if !runnerbinding.SamePath(later.Execution.Workspace, filepath.Join(c.StateDir, "runs", later.RunID, "content")) ||
		later.RepositorySource != nil || later.ContentResult == nil || later.ContentResult.Status != "succeeded" ||
		later.Final == nil || !later.Final.Verified {
		t.Fatal("directory-free generate did not produce a verified result", later)
	}
	if err = VerifyFileMaterials(g.Generate.Materials, later.ContentResult.FileMaterials); err != nil {
		t.Fatal(err)
	}
	input.ReuseWorktreeFromRunID = latest
	body, _ = json.Marshal(input)
	receipt, err = c.Submit(ctx, Request{Protocol: Protocol, RequestID: "maintenance-after-readonly", Input: body})
	if err != nil {
		t.Fatal("readonly transferred writable ownership", err)
	}
	runs = append(runs, receipt.RunID)
	after := wait(receipt.RunID, true)
	if after.WorkspaceIdentity != identity || after.ReuseWorktreeFromRunID != latest {
		t.Fatal("readonly changed original writer chain", after)
	}
	t.Logf("fixed EXE maintenance: four execute Runs share origin=%s identity=%s workspace=%s branch=%s; distinct sessions; dirty file retained without result files; readonly directory/file manifests verified", origin, identity, workspace, branch)
}
