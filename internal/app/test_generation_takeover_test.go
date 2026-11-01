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

func TestTMGenerationOfflineInspectionAndDurableStartupStop(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	require(t, err)
	defer store.Close()
	cli := startupFixtureCLI(t)
	original := cli.BuildPin()
	original.SHA256 = strings.Repeat("a", 64)
	original.Version = "old"
	g := testmanagement.Generation{ID: strings.Repeat("e", 32), Project: "project", Revision: 1, Runner: original, RunID: "original-run", InputDigest: "original-input", Cancelled: true, State: "accepted", Request: runnercontract.Request{Protocol: runnerclient.Protocol, RequestID: "original-submit", Input: json.RawMessage(`{"title":"original"}`)}}
	require(t, store.SaveGeneration(ctx, g, 0))
	writeStartupInspection(t, cli, runnercontract.Run{RunID: g.RunID, InputDigest: g.InputDigest, Revision: 9, Correlation: map[string]string{"steerline_project": g.Project, "steerline_generation": g.ID}}, g.Request.RequestID)
	stops, err := prepareGenerationStops(ctx, store, g.Project, &cli)
	require(t, err)
	if len(stops) != 1 || stops[0].RunID != g.RunID || string(stops[0].Request.Input) != `{"expected_revision":9}` {
		t.Fatal(stops)
	}
	again, err := prepareGenerationStops(ctx, store, g.Project, &cli)
	require(t, err)
	if !reflect.DeepEqual(stops, again) {
		t.Fatal("restart replaced the original stop")
	}
	saved, err := store.Generation(ctx, g.Project, g.ID)
	require(t, err)
	if saved.Runner != original || !reflect.DeepEqual(saved.Request, g.Request) || saved.StopState != "unknown" || saved.StopRequest == nil {
		t.Fatal("original pin/input or startup outbox changed", saved)
	}

}

func TestGQQueuedGenerationNeedsNoRunnerTakeoverOrStop(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	require(t, err)
	defer store.Close()
	g := testmanagement.Generation{ID: strings.Repeat("f", 32), Project: "project", Revision: 1, State: "queued", Submission: "pending", Request: runnercontract.Request{RequestID: "never-sent"}}
	require(t, store.SaveGeneration(ctx, g, 0))
	cli := runnerclient.Client{} // No executable: any Runner call would fail.
	stops, err := prepareGenerationStops(ctx, store, g.Project, &cli)
	require(t, err)
	if len(stops) != 0 {
		t.Fatal(stops)
	}
}
