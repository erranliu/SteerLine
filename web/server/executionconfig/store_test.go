package executionconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/localservice"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func fixture(t *testing.T) (*Store, Config) {
	t.Helper()
	exe, _ := os.Executable()
	raw, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Profile{ID: "local", Name: "本机", Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"executable":"codex.exe","model":"gpt-6-luna","reasoning_effort":"medium"}`)}, Workflow: runnercontract.Workflow{ID: "development", Version: 2}}
	c := Config{Version: 3, Runner: cfg.Pin{Executable: exe, Version: "fixture", SHA256: cfg.Digest(raw)}, DefaultProfile: "local", Profiles: []cfg.Profile{p}}
	s := New(t.TempDir())
	if err = localservice.WritePrivateFile(s.Home, "execution.json", c); err != nil {
		t.Fatal(err)
	}
	s.ReadCatalog = func(context.Context, cfg.Pin) (*catalog, error) {
		raw, err := os.ReadFile("../../../internal/runnerclient/testdata/workflow-capabilities.json")
		if err != nil {
			return nil, err
		}
		var caps struct {
			Workflows []runnercontract.WorkflowChoice `json:"workflows"`
			Schemas   struct {
				Config   json.RawMessage            `json:"run_configuration"`
				Adapters map[string]json.RawMessage `json:"adapters"`
			} `json:"configuration_schemas"`
		}
		if err = json.Unmarshal(raw, &caps); err != nil {
			return nil, err
		}
		return compileCatalog(caps.Schemas.Config, caps.Workflows, caps.Schemas.Adapters["codex-app-server"])
	}
	return s, c
}

func TestGQConcurrencySaveAndValidation(t *testing.T) {
	s, c := fixture(t)
	ctx := context.Background()
	base, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := cfg.GenerateConcurrency(base.Config.MaxGenerateConcurrency); err != nil || got != 5 {
		t.Fatal(got, err)
	}
	for _, limit := range []int{0, -1} {
		c.MaxGenerateConcurrency = &limit
		if _, err = s.Validate(ctx, encode(t, c)); err == nil {
			t.Fatal("invalid concurrency accepted", limit)
		}
	}
	limit := 3
	c.MaxGenerateConcurrency = &limit
	saved, err := s.Save(ctx, base.SavedDigest, encode(t, c))
	if err != nil {
		t.Fatal(err)
	}
	read, err := s.Current()
	if err != nil || read.MaxGenerateConcurrency == nil || *read.MaxGenerateConcurrency != 3 || saved.SavedDigest == base.SavedDigest {
		t.Fatal(read, err)
	}
	if base.Config.MaxGenerateConcurrency != nil {
		t.Fatal("saved value rewrote loaded snapshot")
	}
}
func encode(t *testing.T, c Config) []byte {
	t.Helper()
	r, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func status(t *testing.T, err error, want int) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Status != want {
		t.Fatalf("wanted %d, got %v", want, err)
	}
}
func TestExecutionConfigSave(t *testing.T) {
	t.Run("canceled_before_publication", func(t *testing.T) {
		s, c := fixture(t)
		snap, err := s.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(s.Home, "execution.json")
		old, _ := os.ReadFile(path)
		c.Profiles[0].Name = "canceled"
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err = s.Save(ctx, snap.SavedDigest, encode(t, c)); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		current, _ := os.ReadFile(path)
		if !bytes.Equal(old, current) {
			t.Fatal("canceled save changed published bytes")
		}
		temps, _ := filepath.Glob(filepath.Join(s.Home, ".execution-*"))
		if len(temps) != 0 {
			t.Fatal("temporary files leaked", temps)
		}
	})
	s, c := fixture(t)
	ctx := context.Background()
	snapshot, err := s.Snapshot(ctx)
	if err != nil || !snapshot.Writable {
		t.Fatalf("snapshot %v %+v", err, snapshot)
	}
	old, _ := os.ReadFile(filepath.Join(s.Home, "execution.json"))
	parent, _ := os.Stat(s.Home)
	unchanged, err := s.Save(ctx, snapshot.SavedDigest, encode(t, c))
	if err != nil || unchanged.SavedDigest != snapshot.SavedDigest {
		t.Fatal("no-op save rewrote config", err)
	}
	raw, _ := os.ReadFile(filepath.Join(s.Home, "execution.json"))
	if !bytes.Equal(raw, old) {
		t.Fatal("no-op changed bytes")
	}
	_, err = s.Save(ctx, "", encode(t, c))
	status(t, err, 428)
	c.Profiles[0].Name = "新名称"
	next, err := s.Save(ctx, snapshot.SavedDigest, encode(t, c))
	if err != nil || next.SavedDigest == snapshot.SavedDigest {
		t.Fatal(err)
	}
	_, err = s.Save(ctx, snapshot.SavedDigest, encode(t, c))
	status(t, err, 412)
	info, _ := os.Stat(s.Home)
	if parent.Mode() != info.Mode() {
		t.Fatal("parent permissions changed")
	}
	c.Runner.Version = "other"
	_, err = s.Save(ctx, next.SavedDigest, encode(t, c))
	status(t, err, 400)
}
func TestExecutionConfigConcurrentAndFrozen(t *testing.T) {
	s, c := fixture(t)
	ctx := context.Background()
	snap, _ := s.Snapshot(ctx)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, name := range []string{"one", "two"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			copy := c
			copy.Profiles = append([]cfg.Profile(nil), c.Profiles...)
			copy.Profiles[0].Name = name
			_, e := s.Save(ctx, snap.SavedDigest, encode(t, copy))
			results <- e
		}(name)
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else {
			status(t, e, 412)
			conflict++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal(success, conflict)
	}
	if err := os.WriteFile(filepath.Join(s.Home, "manifest.json"), []byte(`{"version":1,"release":"test","default_scheme":"live","source_commit":"fixture"}`), 0600); err != nil {
		t.Fatal(err)
	}
	snap, _ = s.Snapshot(ctx)
	if snap.Writable {
		t.Fatal("debug scheme is editable")
	}
	_, err := s.Save(ctx, snap.SavedDigest, encode(t, c))
	status(t, err, 409)
}
func TestExecutionConfigSchemaAndPreservation(t *testing.T) {
	s, c := fixture(t)
	ctx := context.Background()
	snapshot, _ := s.Snapshot(ctx)
	for _, settings := range []string{`{"model":"x","reasoning_effort":"invented"}`, `{"model":"x","extra":1}`, `{"model":"x","model":"y"}`} {
		c.Profiles[0].Execution.Settings = json.RawMessage(settings)
		if _, err := s.Validate(ctx, encode(t, c)); err == nil {
			t.Fatal("invalid settings accepted", settings)
		}
	}
	c.Profiles[0].Execution.Settings = json.RawMessage(`{"executable":"codex.exe","model":"manual-unknown-model","reasoning_effort":"high"}`)
	next, err := s.Save(ctx, snapshot.SavedDigest, encode(t, c))
	if err != nil {
		t.Fatal("manual schema-valid model rejected", err)
	}
	c = next.Config
	c.Profiles[0].Execution.Adapter = "orca-terminal"
	c.Profiles[0].Execution.Target.Kind = "local"
	if _, err = s.Validate(ctx, encode(t, c)); err == nil {
		t.Fatal("abandoned chain accepted")
	}
	c = next.Config
	c.Profiles[0].Execution.Settings = json.RawMessage(`{"executable":"new-arbitrary-program","model":"x"}`)
	if _, err = s.Validate(ctx, encode(t, c)); err == nil {
		t.Fatal("new executable accepted")
	}
	// Public schema optional fields survive editing a display name exactly.
	s, c = fixture(t)
	c.Profiles[0].Execution.Settings = json.RawMessage(`{"executable":"codex.exe","model":"x","github_auth":{"app_id":1,"installation_id":2,"private_key_env":"EXAMPLE_KEY"}}`)
	if err = localservice.WritePrivateFile(s.Home, "execution.json", c); err != nil {
		t.Fatal(err)
	}
	snap, _ := s.Snapshot(ctx)
	c.Profiles[0].Name = "rename"
	got, err := s.Save(ctx, snap.SavedDigest, encode(t, c))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got.Config.Profiles[0].Execution.Settings), "EXAMPLE_KEY") {
		t.Fatal("unmodified reference lost")
	}
}
func TestExecutionConfigStatusIsReadOnly(t *testing.T) {
	s, _ := fixture(t)
	s.ReadCatalog = func(context.Context, cfg.Pin) (*catalog, error) {
		t.Fatal("status invoked Runner metadata")
		return nil, nil
	}
	a, err := s.Status()
	if err != nil || a["saved_digest"] == "" {
		t.Fatal(a, err)
	}
}
