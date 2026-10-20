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

func TestTMGenerationTakeoverAndDurableStartupStop(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	require(t, err)
	defer store.Close()
	cli := runnerclient.Client{StateDir: filepath.Join(t.TempDir(), "runner"), SHA256: strings.Repeat("c", 64), BuildVersion: "current", ResourceDigest: strings.Repeat("d", 64)}
	original := cli.BuildPin()
	original.SHA256 = strings.Repeat("a", 64)
	original.Version = "old"
	g := testmanagement.Generation{ID: strings.Repeat("e", 32), Project: "project", Revision: 1, Runner: original, RunID: "original-run", InputDigest: "original-input", Cancelled: true, State: "accepted", Request: runnercontract.Request{Protocol: runnerclient.Protocol, RequestID: "original-submit", Input: json.RawMessage(`{"title":"original"}`)}}
	require(t, store.SaveGeneration(ctx, g, 0))
	proof := runnercontract.TakeoverReport{Contract: "steerrunner-takeover/v1", StateDir: cli.StateDir, CandidateSHA256: cli.SHA256, Compatible: true, Items: []runnercontract.TakeoverItem{{Kind: "run", ID: g.RunID, InputDigest: g.InputDigest, Revision: 9, Status: "compatible"}}}
	cli.Takeover = &proof
	require(t, verifyGenerationTakeover(g, cli))
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
	for _, kind := range []string{"digest", "namespace", "build", "read-only"} {
		bad := proof
		bad.Items = append([]runnercontract.TakeoverItem{}, proof.Items...)
		switch kind {
		case "digest":
			bad.Items[0].InputDigest = "foreign"
		case "namespace":
			bad.StateDir = t.TempDir()
		case "build":
			bad.CandidateSHA256 = "foreign"
		case "read-only":
			bad.Items[0].Status = "read_only"
		}
		cli.Takeover = &bad
		if verifyGenerationTakeover(g, cli) == nil {
			t.Fatalf("accepted %s proof", kind)
		}
	}
	unknown := g
	unknown.RunID = ""
	unknown.InputDigest = ""
	proof.Items[0].RequestID = g.Request.RequestID
	proof.Items[0].RequestDigest = runnercontract.RequestDigest("submit", "", g.Request)
	if _, ok := generationTakeover(proof, unknown); !ok {
		t.Fatal("lost response cannot bind original accepted request")
	}
	unknown.Request.Input = json.RawMessage(`{"title":"replacement"}`)
	if _, ok := generationTakeover(proof, unknown); ok {
		t.Fatal("changed input matched by request ID")
	}
}
