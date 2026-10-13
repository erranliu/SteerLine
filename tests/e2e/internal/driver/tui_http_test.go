package driver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	tea "github.com/charmbracelet/bubbletea"
	app "github.com/erranliu/SteerLine/internal/app"
	"github.com/erranliu/SteerLine/internal/console"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	wire "github.com/erranliu/SteerLine/internal/product/httpcontract"
	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTUIHTTPRuntime(t *testing.T) {
	root := os.Getenv("STEERLINE_TUI_HTTP_TEST_ROOT")
	if root == "" {
		t.Skip("requires scripts/test-tui-http.ps1 and a new isolated package")
	}
	d, err := openDebugRelease(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, "scheme.json")); !os.IsNotExist(err) {
		t.Fatal("runtime requires fresh unfrozen package")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	prepare := d.command(d.helper, "debug", "prepare", "--simulate")
	if out, err := prepare.CombinedOutput(); err != nil {
		t.Fatalf("prepare: %v %s", err, out)
	}
	log, err := os.Create(filepath.Join(root, "evidence", "tui-http-service.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	command := d.command(d.helper, "debug", "run", "--simulate")
	command.Stdout, command.Stderr = log, log
	service, err := testprocess.Start(command)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		if err := service.Stop(cleanup); err != nil {
			t.Errorf("owned process cleanup: %v", err)
		}
	}()
	t.Setenv("STEERLINE_HOME", d.home)
	t.Setenv("STEERLINE_E2E_CONTROLLED_STATE", d.provider)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	wait := func(label string, predicate func() bool) {
		t.Helper()
		t.Log("waiting:", label)
		for ctx.Err() == nil {
			if predicate() {
				return
			}
			select {
			case <-service.Done:
				t.Fatalf("%s: service exited; see %s", label, log.Name())
			default:
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal(label + " timeout")
	}
	var refs []api.ProjectRef
	var options console.Options
	loggedConnection := false
	wait("HTTP service and TUI composition", func() bool {
		var err error
		refs, options, err = app.PrepareConsole(ctx, d.config, "")
		if err != nil || len(refs) != 1 || refs[0].Error != "" {
			if !loggedConnection {
				t.Logf("connection configuration: refs=%+v err=%v", refs, err)
				loggedConnection = true
			}
			return false
		}
		client := options.Clients[refs[0].Key]
		if client == nil {
			return false
		}
		snapshot := client.ReadProject(ctx, "", api.ProjectSnapshot{})
		if snapshot.Connection.State != api.ConnectionConnected {
			if !loggedConnection {
				t.Log("connection:", snapshot.Connection.State, snapshot.Connection.Diagnostic)
				loggedConnection = true
			}
			client.Close()
			return false
		}
		return true
	})
	client, ok := options.Clients[refs[0].Key].(*httpclient.Console)
	if !ok {
		t.Fatal("actual console assembly retained local adapter")
	}
	defer client.Close()
	model, err := console.NewModel(ctx, refs, options)
	if err != nil {
		t.Fatal(err)
	}
	nextSized, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = nextSized.(console.Model)
	refresh := func() {
		batch, ok := model.Init()().(tea.BatchMsg)
		if !ok || len(batch) < 2 {
			t.Fatal("model has no initial query")
		}
		next, _ := model.Update(batch[1]())
		model = next.(console.Model)
	}
	refresh()
	snapshot := client.ReadProject(ctx, "1", api.ProjectSnapshot{})
	if snapshot.SelectedNative == nil {
		t.Fatal("HTTP did not provide selected task")
	}
	revision := snapshot.Connection.Snapshot.Page.Control.Revision
	request, err := client.SubmitControl(ctx, api.ControlInput{Action: api.ResumeDispatch, RequestID: strings.Repeat("1", 32), InstanceID: snapshot.Connection.Snapshot.Page.Service.InstanceID, ExpectedRevision: &revision})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.WaitControl(ctx, request.RequestID, 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	refresh()
	// Start from the real TUI's selected task/profile and confirmation handlers.
	for _, key := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune{'s'}}, {Type: tea.KeyEnter}} {
		next, cmd := model.Update(key)
		model = next.(console.Model)
		if cmd != nil {
			next, _ = model.Update(cmd())
			model = next.(console.Model)
		}
	}
	wait("candidate from TUI command", func() bool {
		snapshot = client.ReadProject(ctx, "1", snapshot)
		return snapshot.Connection.Snapshot.Page != nil && len(snapshot.Connection.Snapshot.Page.Runs) > 0
	})
	run := snapshot.Connection.Snapshot.Page.Runs[0]
	report, err := client.InspectRecovery(ctx, run.RunID)
	if err != nil {
		t.Fatal("public recovery:", err)
	}
	if report.RunID != run.RunID {
		t.Fatal("recovery target changed")
	}
	// Closing the active TUI must leave the worker and service running. Reopen
	// solely through discovery/HTTP, then observe the same original execution.
	if err = model.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := httpclient.NewConsole(ctx, refs[0])
	defer reopened.Close()
	wait("controlled publish", func() bool { state, err := controlled.Read(d.provider); return err == nil && state.PR.State == "open" })
	state, err := controlled.Read(d.provider)
	if err != nil || state.PR.Repository != debugRepository || state.PR.BaseBranch != d.base {
		t.Fatal("provider escaped dedicated debug branch")
	}
	snapshot = reopened.ReadProject(ctx, "1", api.ProjectSnapshot{})
	if snapshot.Connection.State != api.ConnectionConnected || len(snapshot.Connection.Snapshot.Page.Runs) != 1 || snapshot.Connection.Snapshot.Page.Runs[0].RunID != run.RunID {
		t.Fatal("closing TUI stopped service")
	}
	// Duplicate the original HTTP intent after TUI reopen. A fresh business
	// request or execution would fail the subsequent one-run identity assertion.
	files, err := os.ReadDir(filepath.Join(d.home, "ui", "http-requests", wire.ProjectID(refs[0].Identity)))
	if err != nil {
		t.Fatal(err)
	}
	var original wire.CommandEnvelope
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(d.home, "ui", "http-requests", wire.ProjectID(refs[0].Identity), file.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var record struct {
			Envelope wire.CommandEnvelope `json:"envelope"`
		}
		if err = json.Unmarshal(data, &record); err != nil {
			t.Fatal(err)
		}
		if record.Envelope.Action == "start_task" {
			if original.RequestID != "" {
				t.Fatal("multiple start intents")
			}
			original = record.Envelope
		}
	}
	if original.RequestID == "" {
		t.Fatal("TUI start intent not persisted")
	}
	low, err := app.OpenHTTPClient(ctx, d.config)
	if err != nil {
		t.Fatal(err)
	}
	defer low.Close()
	duplicate, err := low.SubmitCommand(ctx, original)
	if err != nil || duplicate.RequestID != original.RequestID || duplicate.Status != "applied" {
		t.Fatalf("duplicate original: %+v %v", duplicate, err)
	}
	page, err := reopened.Conversation(ctx, api.ConversationFilter{WorkflowID: run.RunID, ExecutionID: run.ExecutionID, FinalsOnly: true}, "")
	if err != nil || len(page.Events) == 0 {
		t.Fatalf("canonical final query: %+v %v", page, err)
	}
	bodies := 0
	for _, event := range page.Events {
		if event.Body == nil {
			continue
		}
		body, err := reopened.Body(ctx, *event.Body, 0, 256<<10)
		if err != nil || len(body.Data) == 0 || body.SHA256 != event.Body.SHA256 || body.Revision != event.Body.Revision {
			t.Fatalf("canonical body: %+v %v", body, err)
		}
		bodies++
	}
	if bodies == 0 {
		t.Fatal("no canonical final body asserted")
	}
	snapshot = reopened.ReadProject(ctx, "1", snapshot)
	if len(snapshot.Connection.Snapshot.Page.Runs) != 1 || snapshot.Connection.Snapshot.Page.Runs[0].ExecutionID != run.ExecutionID {
		t.Fatal("duplicate/reopen created a replacement execution")
	}
	result, err := reopened.StopService(ctx, "test", 30*time.Second)
	if err != nil || result.Status != "stopped" || !result.ProcessExited || !result.LocksReleased {
		t.Fatalf("stop proof: %+v %v", result, err)
	}
	select {
	case <-service.Done:
	case <-time.After(10 * time.Second):
		t.Fatal("service did not exit")
	}
	if err = service.Stop(context.Background()); err != nil {
		t.Fatal("cleanup proof", err)
	}
	evidence := os.Getenv("STEERLINE_TUI_HTTP_EVIDENCE")
	if evidence == "" {
		t.Fatal("missing evidence directory")
	}
	binary, err := os.ReadFile(d.binary)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(binary)
	manifest, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var source struct {
		Commit string `json:"source_commit"`
		Dirty  bool   `json:"source_dirty"`
	}
	if err = json.Unmarshal(manifest, &source); err != nil {
		t.Fatal(err)
	}
	summary := map[string]any{"passed": true, "complete": true, "cleaned": true, "scenario": "tui-http.local", "root": root, "project": refs[0].Identity, "run": run.RunID, "execution": run.ExecutionID, "attempt": run.AttemptID, "start_request": original.RequestID, "final_bodies": bodies, "stop": result, "mode": "simulate", "workflow_turns": len(state.Turns), "source_commit": source.Commit, "source_dirty": source.Dirty, "binary_sha256": hex.EncodeToString(hash[:]), "cleanup": "owned Job has no remaining descendants; debug package retained as evidence"}
	data, _ := json.MarshalIndent(summary, "", "  ")
	if err = os.WriteFile(filepath.Join(evidence, "summary.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
