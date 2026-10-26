package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

func TestProjectEnvironmentExecutionLifecycleTakeoverAndStartupStop(t *testing.T) {
	for _, kind := range []string{"adaptation", "generation"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			store, e := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
			require(t, e)
			defer store.Close()
			cli := runnerclient.Client{StateDir: filepath.Join(t.TempDir(), "runner"), SHA256: strings.Repeat("c", 64), BuildVersion: "current", ResourceDigest: strings.Repeat("d", 64)}
			old := cli.BuildPin()
			old.SHA256 = strings.Repeat("a", 64)
			r := env.Record{ID: env.ID(), Project: "project", Revision: 1, Kind: kind, Build: old, Cancelled: true, State: "stopping", CreatedAt: time.Now().UTC(), Requests: []env.Request{{ID: env.ID(), Original: json.RawMessage(`{}`)}}, Rounds: []env.Round{{Number: 1, Submission: "accepted", RunID: "original-run", InputDigest: "original-input", Request: runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: "original-submit", Input: json.RawMessage(`{"title":"original"}`)}}}}
			require(t, store.SaveEnvironment(ctx, r, 0))
			proof := runnercontract.TakeoverReport{Contract: "steerrunner-takeover/v1", StateDir: cli.StateDir, CandidateSHA256: cli.SHA256, Compatible: true, Items: []runnercontract.TakeoverItem{{Kind: "run", ID: "original-run", InputDigest: "original-input", Revision: 9, Status: "compatible"}}}
			cli.Takeover = &proof
			c := Config{identity: "project", runtimeGenerations: testmanagement.NewGenerationQueue(1)}
			stops, e := prepareEnvironmentStops(ctx, c, store, &cli)
			require(t, e)
			again, e := prepareEnvironmentStops(ctx, c, store, &cli)
			require(t, e)
			if len(stops) != 1 || !reflect.DeepEqual(stops, again) || string(stops[0].Request.Input) != `{"expected_revision":9}` {
				t.Fatal("original stop lost", stops, again)
			}
			saved, e := store.Environment(ctx, r.Project, r.ID)
			require(t, e)
			if saved.Build != old || !reflect.DeepEqual(saved.Rounds[0].Request, r.Rounds[0].Request) || saved.Rounds[0].StopState != "unknown" {
				t.Fatal("original pin/input rewritten")
			}
			for _, change := range []func(*runnercontract.TakeoverReport){func(p *runnercontract.TakeoverReport) { p.Items[0].InputDigest = "foreign" }, func(p *runnercontract.TakeoverReport) { p.StateDir = t.TempDir() }, func(p *runnercontract.TakeoverReport) { p.Items[0].Status = "read_only" }} {
				bad := proof
				bad.Items = append([]runnercontract.TakeoverItem{}, proof.Items...)
				change(&bad)
				cli.Takeover = &bad
				if _, e := prepareEnvironmentStops(ctx, c, store, &cli); e == nil {
					t.Fatal("accepted foreign original execution")
				}
			}
			a := r.Rounds[0]
			a.RunID = ""
			a.InputDigest = ""
			proof.Items[0].RequestID = a.Request.RequestID
			proof.Items[0].RequestDigest = runnercontract.RequestDigest("submit", "", a.Request)
			if _, ok := environmentTakeover(proof, r, a); !ok {
				t.Fatal("lost submit receipt cannot bind original request")
			}
			a.Request.Input = json.RawMessage(`{"title":"replacement"}`)
			if _, ok := environmentTakeover(proof, r, a); ok {
				t.Fatal("changed original input matched")
			}
		})
	}
}
