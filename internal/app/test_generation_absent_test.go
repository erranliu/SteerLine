package app

import (
	"context"
	"encoding/json"
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
			cli := runnerclient.Client{StateDir: filepath.Join(t.TempDir(), "runner"), SHA256: strings.Repeat("a", 64), BuildVersion: "current", ResourceDigest: strings.Repeat("b", 64)}
			g := testmanagement.Generation{ID: "original-generation", Project: "project", Revision: 1, Runner: cli.BuildPin(), Cancelled: true, State: "cancelled", Submission: "started", SubmissionDiagnostic: "original submit response unavailable", StopID: "original-stop", Request: runnercontract.Request{Protocol: runnerclient.Protocol, RequestID: "original-submit", Input: json.RawMessage(`{"title":"original"}`)}}
			proof := runnercontract.TakeoverReport{Contract: "steerrunner-takeover/v1", StateDir: cli.StateDir, CandidateSHA256: cli.SHA256, Compatible: true, Items: []runnercontract.TakeoverItem{}}
			switch kind {
			case "accepted-original":
				proof.Items = []runnercontract.TakeoverItem{{Kind: "run", ID: "original-run", RequestID: g.Request.RequestID, RequestDigest: runnercontract.RequestDigest("submit", "", g.Request), Revision: 9, InputDigest: "original-input", Status: "compatible"}}
			case "online":
				proof.InstanceID = "still-running"
			case "active":
				g.Cancelled, g.State = false, "unknown"
			case "namespace":
				g.Runner.StateDir = t.TempDir()
			case "bound-run-missing":
				g.RunID, g.InputDigest = "known-run", "known-input"
			case "request-conflict":
				proof.Items = []runnercontract.TakeoverItem{{Kind: "run", ID: "foreign-run", RequestID: g.Request.RequestID, RequestDigest: "foreign-digest", Revision: 1, InputDigest: "foreign-input", Status: "compatible"}}
			case "incomplete-report":
				proof.Items = []runnercontract.TakeoverItem{{Kind: "run", ID: "unbound-run", Revision: 1, InputDigest: "input", Status: "compatible"}}
			}
			require(t, store.SaveGeneration(ctx, g, 0))
			cli.Takeover = &proof
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
