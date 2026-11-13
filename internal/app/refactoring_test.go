package app

import (
	"context"
	"encoding/json"
	em "github.com/erranliu/SteerLine/internal/execution/model"
	ref "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	rc "github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRefactoringPRReportAndOriginalHead(t *testing.T) {
	for _, kind := range []string{"valid", "fork", "closed", "body", "head", "base"} {
		t.Run(kind, func(t *testing.T) {
			head := strings.Repeat("a", 40)
			body := "完整报告"
			base := strings.Repeat("b", 40)
			if kind == "base" {
				base = ""
			}
			fork := "owner/repo"
			state := "open"
			if kind == "fork" {
				fork = "other/repo"
			}
			if kind == "closed" {
				state = "closed"
			}
			if kind == "body" {
				body = "不同正文"
			}
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"number": 1, "html_url": server.URL + "/owner/repo/pulls/1", "body": body, "state": state, "merged": false, "base": map[string]any{"ref": "main", "sha": base, "repo": map[string]string{"full_name": "owner/repo"}}, "head": map[string]any{"ref": "codex/maintenance", "sha": head, "repo": map[string]string{"full_name": fork}}})
			}))
			defer server.Close()
			p := refactoringPort{config: Config{PR: em.PRProviderConfig{Kind: "gitea", BaseURL: server.URL, Repository: "owner/repo"}, CommandTimeout: "3s"}}
			record := ref.Record{Frozen: ref.Frozen{Repository: rc.Repository{BaseBranch: "main"}, Publication: map[string]string{"repository": "owner/repo", "base_url": server.URL}}, Operations: []ref.Operation{{Run: &rc.Run{Execution: rc.Execution{Branch: "codex/maintenance"}}}}}
			result := ref.Result{Head: head, PR: server.URL + "/owner/repo/pulls/1"}
			if kind == "head" {
				result.Head = strings.Repeat("c", 40)
			}
			fact, gotBody, e := p.verify(context.Background(), record, result)
			if (e == nil) != (kind == "valid" || kind == "body" || kind == "head") {
				t.Fatal(kind, fact, e)
			}
			if e == nil && (fact.BaseRevision != base || fact.HeadRevision != head || gotBody != body) {
				t.Fatal("version query must return the actual platform facts")
			}
		})
	}
}

func TestRefactoringStartupReconcilesOriginalStop(t *testing.T) {
	for _, kind := range []string{"missing-stale", "missing-current", "accepted", "rejected", "foreign", "unavailable", "stopped", "initialization-failed"} {
		t.Run(kind, func(t *testing.T) {
			c := Config{identity: "project"}
			cli := startupFixtureCLI(t)
			ctx := context.Background()
			store, e := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
			require(t, e)
			defer store.Close()
			id := strings.Repeat("a", 32)
			op := ref.Operation{ID: strings.Repeat("b", 32), Purpose: "implement", State: "submitted", RunID: "original-run", InputDigest: "original-input", Request: rc.Request{Protocol: rc.Protocol, RequestID: "original-submit", Input: json.RawMessage(`{}`)}, Stop: runnerbinding.ContentStop("original-stop", 3), StopState: "unknown"}
			run := rc.Run{RunID: op.RunID, Revision: 4, InputDigest: op.InputDigest, Workflow: rc.Workflow{ID: "execute", Version: 1}, Correlation: map[string]string{"steerline_project": c.Identity(), "steerline_refactoring": id, "refactoring_operation": op.ID}, Status: "running", StopStatus: "active"}
			if kind == "missing-current" {
				run.Revision = 3
			}
			if kind == "stopped" {
				run.StopStatus = "stopped"
			}
			const initializationDiagnostic = "required result_file initialization failed: initialization_timeout"
			if kind == "initialization-failed" {
				op.Purpose = "publish"
				run.Status, run.Phase, run.StopStatus = "blocked", "result_file_initialization_failed", ""
				run.Diagnostic = initializationDiagnostic
				op.Stop, op.StopState = nil, ""
			}
			entries := map[string]any{run.RunID: rc.Inspection{Run: &run}}
			if kind == "accepted" || kind == "rejected" || kind == "foreign" {
				target := run.RunID
				if kind == "foreign" {
					target = "foreign"
				}
				status := "pending"
				if kind == "rejected" {
					status = "rejected"
				}
				entries["original-stop"] = rc.Inspection{Run: &run, Command: &rc.Command{RequestID: "original-stop", Command: "stop", RunID: &target, ReceiptStatus: "accepted", ActionStatus: status}}
			}
			if kind == "unavailable" {
				entries["original-stop"] = map[string]string{"fixture_error": "service_unavailable"}
			}
			require(t, os.WriteFile(filepath.Join(cli.StateDir, "idle-inspections-fixture.json"), mustJSON(entries), 0600))
			record := ref.Record{ID: id, Project: c.Identity(), Revision: 1, Phase: "execute", Frozen: ref.Frozen{Runner: cli.BuildPin()}, Operations: []ref.Operation{op}, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
			require(t, store.SaveRefactoring(ctx, record, 0, nil))
			stops, e := prepareRefactoringStops(ctx, &c, store, &cli)
			if kind == "foreign" || kind == "unavailable" {
				if e == nil {
					t.Fatal("unverified stop accepted")
				}
				return
			}
			require(t, e)
			if kind == "stopped" {
				if len(stops) != 0 {
					t.Fatal(stops)
				}
				return
			}
			if len(stops) != 1 {
				t.Fatal(stops)
			}
			changed := kind == "missing-stale" || kind == "rejected" || kind == "initialization-failed"
			if (stops[0].Request.RequestID != "original-stop") != changed {
				t.Fatal("wrong original stop decision", kind, stops)
			}
			saved, e := store.Refactoring(ctx, c.Identity(), id)
			require(t, e)
			if kind == "initialization-failed" {
				if saved.Operations[0].InitializationFailure != initializationDiagnostic {
					t.Fatal("startup did not save the original initialization failure before stopping")
				}
				run.Revision++
				run.Status, run.Phase, run.StopStatus, run.Diagnostic = "stopped", "stopped", "stopped", ""
				require(t, os.WriteFile(filepath.Join(cli.StateDir, "idle-inspections-fixture.json"), mustJSON(entries), 0600))
				again, err := prepareRefactoringStops(ctx, &c, store, &cli)
				require(t, err)
				saved, err = store.Refactoring(ctx, c.Identity(), id)
				require(t, err)
				if len(again) != 0 || saved.Operations[0].Run.Phase != "stopped" || saved.Operations[0].InitializationFailure != initializationDiagnostic {
					t.Fatal("stopped observation replaced the original failure or issued another stop")
				}
				return
			}
			if changed && len(saved.Operations[0].PriorStops) != 1 {
				t.Fatal("old stop evidence lost")
			}
			again, e := prepareRefactoringStops(ctx, &c, store, &cli)
			require(t, e)
			if len(again) != 1 || again[0].Request.RequestID != stops[0].Request.RequestID {
				t.Fatal("second startup changed unresolved original", again)
			}
		})
	}
}
