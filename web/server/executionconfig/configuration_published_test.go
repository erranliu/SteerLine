//go:build runneracceptance

package executionconfig

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/localservice"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestExecutionConfigurationPublishedStore(t *testing.T) {
	ctx := context.Background()
	p := cfg.Profile{ID: "saved", Name: "Saved", Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"access_mode":"full_access","executable":"C:/trusted/custom-codex.exe","model":"original","reasoning_effort":"high"}`)}, Workflow: runnercontract.Workflow{ID: "development", Version: 2}}
	current := Config{Version: 3, Runner: cfg.Pin{Executable: os.Getenv("STEERRUNNER_EXE"), Version: os.Getenv("STEERRUNNER_VERSION"), SHA256: os.Getenv("STEERRUNNER_SHA256")}, DefaultProfile: p.ID, Profiles: []cfg.Profile{p}}
	s := New(t.TempDir())
	if err := localservice.WritePrivateFile(s.Home, "execution.json", current); err != nil {
		t.Fatal(err)
	}
	snap, err := s.Snapshot(ctx)
	if err != nil || !snap.Profiles[p.ID].Editable {
		t.Fatal(snap, err)
	}
	selection := *snap.Profiles[p.ID].View
	model := "changed"
	selection.Model = &model
	selection.ReasoningEffort = nil
	request := ComposeRequest{SavedDigest: snap.SavedDigest, BaseProfileID: p.ID, Selection: selection, Unset: []string{"reasoning_effort"}, Workflow: p.Workflow}
	composed, err := s.Compose(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	proposed := current
	proposed.Profiles = append([]cfg.Profile(nil), current.Profiles...)
	proposed.Profiles[0].Execution = *composed.Execution
	edits := []ProfileEdit{{ProfileID: p.ID, BaseProfileID: p.ID, Selection: selection, Unset: request.Unset}}
	encode := func(c Config, e []ProfileEdit) []byte {
		raw, _ := json.Marshal(EditRequest{SavedDigest: snap.SavedDigest, Config: c, ProfileEdits: e})
		return raw
	}
	if _, err = s.Validate(ctx, encode(proposed, nil)); err == nil {
		t.Fatal("raw execution authorized without logical edit")
	}
	for _, bad := range [][]ProfileEdit{append(append([]ProfileEdit{}, edits...), edits[0]), {{ProfileID: "unknown", Selection: selection}}, {{ProfileID: p.ID, BaseProfileID: "missing", Selection: selection}}} {
		if _, err = s.Validate(ctx, encode(proposed, bad)); err == nil {
			t.Fatal("invalid edit coverage accepted")
		}
	}
	forged := proposed
	forged.Profiles = append([]cfg.Profile(nil), proposed.Profiles...)
	forged.Profiles[0].Execution.Settings = json.RawMessage(`{"access_mode":"full_access","executable":"C:/injected/evil.exe","model":"changed"}`)
	if _, err = s.Validate(ctx, encode(forged, edits)); err == nil {
		t.Fatal("program injection accepted")
	}
	if _, err = s.Validate(ctx, encode(proposed, edits)); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Save(ctx, snap.SavedDigest, encode(proposed, edits))
	if err != nil {
		t.Fatal(err)
	}
	if !sameValue(saved.Config.Profiles[0].Execution, *composed.Execution) {
		t.Fatal("save changed Runner result")
	}
	if _, err = s.Compose(ctx, request); err == nil {
		t.Fatal("stale saved_digest accepted")
	}
	t.Logf("fixed Runner Web compose/validate/save: version=%s sha256=%s resource=%s", current.Runner.Version, current.Runner.SHA256, s.catalog.ResourceDigest)
}
