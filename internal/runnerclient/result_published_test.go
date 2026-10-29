package runnerclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// One caller/public-EXE boundary test. The native protocol fixture is supplied
// by Runner; this does not duplicate its collector matrix or prove a real PR.
func TestProjectEnvironmentExecutionPublishedResultRoundTrip(t *testing.T) {
	exe, native := os.Getenv("STEERRUNNER_EXE"), os.Getenv("STEERRUNNER_NATIVE_FIXTURE")
	if exe == "" || native == "" {
		t.Skip("explicit pinned Runner and its native protocol fixture required")
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
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
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
	t.Setenv("CODEX_HOME", state)
	t.Setenv("STEERRUNNER_FIXTURE_STATE", state)
	// This contract exercises collection during a held native Turn. It does
	// not ask the protocol fixture to recognize or implement business prompts.
	t.Setenv("STEERRUNNER_CONTENT_FIXTURE", "1")
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
	check(os.WriteFile(filepath.Join(source, "spec.md"), []byte("Controlled caller result contract"), 0600))
	check(os.WriteFile(filepath.Join(source, "plan.md"), []byte("Hold native fixture; caller stops original Run"), 0600))
	git(source, "add", ".")
	git(source, "-c", "user.name=Caller Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "fixture")
	commit := git(source, "rev-parse", "HEAD")
	git(root, "config", "--global", "url."+filepath.ToSlash(bare)+".insteadOf", "https://controlled.invalid/owner/repo.git")
	git(source, "remote", "add", "origin", "https://controlled.invalid/owner/repo.git")
	git(source, "push", "origin", "main")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	selection := Selection{Executable: exe, Version: os.Getenv("STEERRUNNER_VERSION"), SHA256: os.Getenv("STEERRUNNER_SHA256")}
	c, version, err := selection.Open(ctx, filepath.Join(root, "runner"), 30*time.Second)
	check(err)
	info, err := c.StartService(ctx, version)
	check(err)
	var runID string
	var stop *Request
	// Cleanup only addresses the original Run/request and original service.
	stopRun := func() error {
		v, err := c.Inspect(context.Background(), "run-id", runID, false)
		if err != nil || v.Run.StopStatus == "stopped" {
			return err
		}
		if stop == nil {
			body, _ := json.Marshal(map[string]uint64{"expected_revision": v.Run.Revision})
			stop = &Request{Protocol: Protocol, RequestID: "caller-result-stop", Input: body}
			_, err = c.Call(context.Background(), "stop", []string{"--run-id", runID}, stop)
		}
		return err
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
	settings, _ := json.Marshal(map[string]string{"executable": native})
	publication, _ := json.Marshal(map[string]any{"base_url": "https://controlled.invalid", "query": map[string]any{"executable": native, "args": []string{"--query", provider}}})
	input := runnercontract.RunInput{Title: "Caller result contract", Repository: runnercontract.Repository{URL: "https://controlled.invalid/owner/repo.git", LocalPath: source, Remote: "origin", BaseBranch: "main", BaseRevision: commit}, Documents: runnercontract.Documents{Spec: runnercontract.Document{Path: "spec.md"}, Plan: runnercontract.Document{Path: "plan.md"}}, Workflow: runnercontract.Workflow{ID: "development", Version: 2}, Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: settings}, Publication: runnercontract.Publication{Provider: "command", Repository: "owner/repo", BaseBranch: "main", Settings: publication}, ProgressFile: &runnercontract.ProgressFileInput{InitialContentBase64: base64.StdEncoding.EncodeToString([]byte(`{"test.one":false}`))}, ResultFile: &runnercontract.ProgressFileInput{InitialContentBase64: base64.StdEncoding.EncodeToString([]byte("seed"))}}
	body, _ := json.Marshal(input)
	receipt, err := c.Submit(ctx, Request{Protocol: Protocol, RequestID: "caller-result-submit", Input: body})
	check(err)
	runID = receipt.RunID
	var run *Run
	for ctx.Err() == nil {
		v, err := c.Inspect(ctx, "run-id", runID, false)
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
		v, err := c.Inspect(ctx, "run-id", runID, false)
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
}
