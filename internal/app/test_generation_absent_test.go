package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

func TestTMGenerationCancelledMissingSubmissionRequiresOfflineProof(t *testing.T) {
	for _, kind := range []string{"absent", "accepted-original", "online", "active", "namespace", "bound-run-missing", "request-conflict", "incomplete-report"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
			require(t, err)
			defer store.Close()
			cli := startupFixtureCLI(t)
			g := testmanagement.Generation{ID: "original-generation", Project: "project", Revision: 1, Runner: cli.BuildPin(), Cancelled: true, State: "cancelled", Submission: "started", SubmissionDiagnostic: "original submit response unavailable", StopID: "original-stop", Request: runnercontract.Request{Protocol: runnerclient.Protocol, RequestID: "original-submit", Input: json.RawMessage(`{"title":"original"}`)}}
			inspection := map[string]any{"fixture_error": "request_not_found"}
			switch kind {
			case "accepted-original":
				id := "original-run"
				inspection = map[string]any{"run": runnercontract.Run{RunID: id, InputDigest: "original-input", Revision: 9, Correlation: map[string]string{"steerline_project": g.Project, "steerline_generation": g.ID}}, "command": runnercontract.Command{Command: "submit", RequestID: g.Request.RequestID, RunID: &id}}
			case "online":
				cli.ResourceDigest = strings.Repeat("a", 64)
				require(t, os.WriteFile(filepath.Join(cli.StateDir, "idle-fixture.json"), mustJSON(runnerclient.ServiceInfo{Protocol: runnerclient.Protocol, StateDir: cli.StateDir, Status: "ready", InstanceID: "online", Version: cli.BuildVersion, ResourceDigest: cli.ResourceDigest, ExecutableSHA256: cli.SHA256}), 0600))
			case "active":
				g.Cancelled, g.State = false, "unknown"
			case "namespace":
				g.Runner.StateDir = t.TempDir()
			case "bound-run-missing":
				g.RunID, g.InputDigest = "known-run", "known-input"
			case "request-conflict":
				inspection = map[string]any{"fixture_error": "request_conflict"}
			case "incomplete-report":
				inspection = map[string]any{"run": runnercontract.Run{RunID: "foreign", Revision: 1}}
			}
			require(t, os.WriteFile(filepath.Join(cli.StateDir, "idle-run-fixture.json"), mustJSON(inspection), 0600))
			require(t, store.SaveGeneration(ctx, g, 0))
			stops, err := prepareGenerationStops(ctx, store, g.Project, &cli)
			if kind == "absent" {
				require(t, err)
				if len(stops) != 0 {
					t.Fatal("created a stop for a nonexistent Run", stops)
				}
				saved, readErr := store.Generation(ctx, g.Project, g.ID)
				require(t, readErr)
				if saved.State != "rejected" || !saved.Cancelled || !saved.Settled() || saved.SubmissionDiagnostic != g.SubmissionDiagnostic || !reflect.DeepEqual(saved.Request, g.Request) || saved.Runner != g.Runner || saved.StopID != g.StopID {
					t.Fatal("offline absence did not settle the original cancellation without changing its identity", saved.State)
				}
				_, err = prepareGenerationStops(ctx, store, g.Project, &cli)
				require(t, err)
				again, _ := store.Generation(ctx, g.Project, g.ID)
				if !reflect.DeepEqual(saved, again) {
					t.Fatal("startup rewrote settled history")
				}
			} else if kind == "accepted-original" {
				require(t, err)
				saved, readErr := store.Generation(ctx, g.Project, g.ID)
				require(t, readErr)
				if len(stops) != 1 || stops[0].RunID != "original-run" || saved.State == "rejected" || saved.Settled() || saved.StopRequest == nil || string(saved.StopRequest.Input) != `{"expected_revision":9}` || !reflect.DeepEqual(saved.Request, g.Request) {
					t.Fatal("late accepted original was treated as absent instead of receiving its original stop", saved.State, stops)
				}
			} else {
				saved, readErr := store.Generation(ctx, g.Project, g.ID)
				require(t, readErr)
				if !reflect.DeepEqual(saved, g) {
					t.Fatal("insufficient proof settled or changed original work", kind, err)
				}
			}
		})
	}
}
