package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/mainagentbridge"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

func TestMainAgentAdvisoryStartupReconcilesOriginalStop(t *testing.T) {
	for _, kind := range []string{"missing-stale", "missing-current", "accepted", "foreign", "unavailable", "already-stopped"} {
		t.Run(kind, func(t *testing.T) {
			c, _ := nativeTaskFixture(t)
			cli := startupFixtureCLI(t)
			ctx := context.Background()
			store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
			require(t, err)
			defer store.Close()
			r := mainagentbridge.Record{ID: strings.Repeat("a", 32), Project: c.Identity(), Source: mainagentbridge.Source{CandidateID: strings.Repeat("b", 32)}, Runner: cli.BuildPin(), Revision: 1, RunID: "original-run", RunnerDigest: "original-input", RunnerRequest: &runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: "original-submit", Input: []byte(`{}`)}, Submission: "started", State: "cancelling", Cancelled: true, CreatedAt: time.Now().UTC(), StopRequest: runnerbinding.ContentStop("original-stop", 3), StopState: "unknown"}
			run := runnercontract.Run{RunID: r.RunID, Revision: 4, InputDigest: r.RunnerDigest, Workflow: runnercontract.Workflow{ID: "generate", Version: 1}, Correlation: map[string]string{"steerline_project": r.Project, "steerline_advisory": r.ID}, Status: "running", StopStatus: "active"}
			if kind == "missing-current" {
				run.Revision = 3
			}
			if kind == "already-stopped" {
				run.StopStatus = "stopped"
			}
			entries := map[string]any{run.RunID: runnercontract.Inspection{Run: &run}}
			if kind == "accepted" || kind == "foreign" {
				target := run.RunID
				if kind == "foreign" {
					target = "foreign"
				}
				entries["original-stop"] = runnercontract.Inspection{Run: &run, Command: &runnercontract.Command{CommandID: "stop-command", RequestID: "original-stop", Command: "stop", RunID: &target, ReceiptStatus: "accepted", ActionStatus: "pending"}}
			}
			if kind == "unavailable" {
				entries["original-stop"] = map[string]string{"fixture_error": "service_unavailable"}
			}
			require(t, os.WriteFile(filepath.Join(cli.StateDir, "idle-inspections-fixture.json"), mustJSON(entries), 0600))
			require(t, store.SaveAdvisory(ctx, r.Project, r.ID, r.Source.CandidateID, mustJSON(r), 0))
			stops, err := prepareAdvisoryStops(ctx, &c, store, &cli)
			if kind == "foreign" || kind == "unavailable" {
				if err == nil {
					t.Fatal("unverified stop allowed", stops)
				}
				return
			}
			require(t, err)
			if kind == "already-stopped" {
				if len(stops) != 0 {
					t.Fatal("stopped Run received another stop", stops)
				}
				return
			}
			if len(stops) != 1 {
				t.Fatal(stops)
			}
			if kind == "missing-stale" {
				if stops[0].Request.RequestID == "original-stop" || string(stops[0].Request.Input) != `{"expected_revision":4}` {
					t.Fatal(stops)
				}
			} else if stops[0].Request.RequestID != "original-stop" {
				t.Fatal("replaced unresolved or accepted original", stops)
			}
			rows, err := advisoryRecords(ctx, store, r.Project)
			require(t, err)
			if rows[0].StopRequest.RequestID != stops[0].Request.RequestID {
				t.Fatal("startup stop not durable", rows)
			}
		})
	}
}
