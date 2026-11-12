package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	protocol "github.com/erranliu/SteerLine/internal/testkit/protocol"
)

func TestProjectEnvironmentSameBuildStartupStops(t *testing.T) {
	for _, scenario := range []string{"new-stop", "original-stop", "absent-stale-stop", "absent-current-stop", "unknown-submit", "cold-unaccepted-submit", "warm-unaccepted-submit", "cold-preparing-submit", "warm-preparing-submit", "warm-rejected-submit", "foreign-input", "foreign-correlation", "foreign-build", "foreign-stop", "already-stopped", "closed-pending", "closed-rejected-stop"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
			require(t, err)
			defer store.Close()
			build := runnercontract.BuildPin{StateDir: filepath.Join(t.TempDir(), "runner"), SHA256: strings.Repeat("c", 64), Version: "current", ResourceDigest: strings.Repeat("d", 64)}
			r := env.Record{FormatVersion: 1, ID: env.ID(), Project: "project", Revision: 1, Kind: "adaptation", Build: build, Cancelled: true, State: "stopping", CreatedAt: time.Now().UTC(), Requests: []env.Request{{ID: env.ID(), Original: json.RawMessage(`{}`)}}, Rounds: []env.Round{{Number: 1, Submission: "accepted", RunID: "original-run", InputDigest: "original-input", Run: &runnercontract.Run{RunID: "original-run", Revision: 3, InputDigest: "original-input"}, Request: runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: "original-submit", Input: json.RawMessage(`{"title":"original"}`)}}}}
			if scenario == "foreign-build" {
				r.Build.StateDir += "-foreign"
			}
			if strings.Contains(scenario, "submit") {
				r.Rounds[0].RunID = ""
				r.Rounds[0].InputDigest = ""
				r.Rounds[0].Run = nil
				r.Rounds[0].Submission = "unknown"
			}
			if strings.Contains(scenario, "stop") && scenario != "new-stop" && scenario != "already-stopped" {
				revision := 3
				if scenario == "absent-current-stop" {
					revision = 9
				}
				body, _ := json.Marshal(map[string]int{"expected_revision": revision})
				r.Rounds[0].Stop = &runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: "original-stop", Input: body}
				r.Rounds[0].StopState = "unknown"
			}
			if scenario == "closed-rejected-stop" {
				r.Rounds[0].StopState = "rejected"
			}
			r.Execution = runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"access_mode":"full_access"}`)}
			r.Prepared.Identity = protocol.ExecutionIdentity(r.Execution)
			r.Prepared.DerivedExecution = r.Execution
			r.Prepared.Input.Execution = r.Execution
			require(t, store.SaveEnvironment(ctx, r, 0))
			run := runnercontract.Run{RunID: "original-run", Revision: 9, InputDigest: "original-input", Status: "running", Correlation: map[string]string{"steerline_project": r.Project, "environment_activity": r.ID, "environment_round": "1"}}
			if scenario == "foreign-input" {
				run.InputDigest = "other"
			}
			if scenario == "foreign-correlation" {
				run.Correlation["environment_activity"] = "other"
			}
			if scenario == "already-stopped" {
				run.Status = "completed"
				run.StopStatus = "stopped"
				run.Closed = true
			}
			if strings.HasPrefix(scenario, "closed-") {
				run.Closed, run.StopStatus = true, "pending"
			}
			calls := 0
			query := func(_ context.Context, selector, id string) (runnerclient.Inspection, bool, error) {
				calls++
				missing := func() (runnerclient.Inspection, bool, error) {
					return runnerclient.Inspection{}, scenario != "warm-unaccepted-submit", &runnerclient.Failure{Code: "request_not_found", Exit: 5}
				}
				if selector == "run-id" {
					if id != run.RunID {
						t.Fatal(id)
					}
					return runnerclient.Inspection{Run: &run}, true, nil
				}
				if id == "original-submit" {
					if strings.Contains(scenario, "unaccepted") {
						return missing()
					}
					if strings.Contains(scenario, "preparing") {
						return runnerclient.Inspection{Command: &runnerclient.Command{RequestID: id, Command: "submit", ReceiptStatus: "pending", ActionStatus: "preparing"}}, strings.HasPrefix(scenario, "cold-"), nil
					}
					if scenario == "warm-rejected-submit" {
						return runnerclient.Inspection{Command: &runnerclient.Command{RequestID: id, Command: "submit", ReceiptStatus: "rejected", ActionStatus: "rejected", Error: &runnercontract.CommandError{Code: "invalid_state", Message: "preparation interrupted"}}}, false, nil
					}
					return runnerclient.Inspection{Run: &run, Command: &runnerclient.Command{RequestID: id, Command: "submit", RunID: &run.RunID}}, true, nil
				}
				if scenario == "original-stop" || scenario == "foreign-stop" {
					target := run.RunID
					if scenario == "foreign-stop" {
						target = "foreign"
					}
					return runnerclient.Inspection{Command: &runnerclient.Command{RequestID: id, Command: "stop", RunID: &target, ActionStatus: "pending", ReceiptStatus: "accepted"}}, true, nil
				}
				return missing()
			}
			c := Config{identity: r.Project}
			stops, err := prepareEnvironmentStopsWith(ctx, c, store, build, query)
			bad := strings.HasPrefix(scenario, "foreign") || scenario == "warm-unaccepted-submit" || scenario == "warm-preparing-submit"
			if bad {
				if err == nil {
					t.Fatal("unverified original accepted")
				}
				if scenario == "foreign-build" && calls != 0 {
					t.Fatal("queried another build")
				}
				return
			}
			require(t, err)
			saved, err := store.Environment(ctx, r.Project, r.ID)
			require(t, err)
			if scenario == "cold-unaccepted-submit" || scenario == "cold-preparing-submit" || scenario == "warm-rejected-submit" {
				if len(stops) != 0 || saved.State != "cancelled" || !saved.Rounds[0].Settled {
					t.Fatal(saved)
				}
				return
			}
			if scenario == "already-stopped" || strings.HasPrefix(scenario, "closed-") {
				if len(stops) != 0 || saved.Rounds[0].Settled {
					t.Fatal("startup forged final report settlement", saved)
				}
				if !reflect.DeepEqual(saved.Rounds[0].Stop, r.Rounds[0].Stop) || saved.Rounds[0].StopState != r.Rounds[0].StopState {
					t.Fatal("closed run replaced original stop history")
				}
				return
			}
			if len(stops) != 1 || stops[0].RunID != run.RunID {
				t.Fatal(stops)
			}
			if scenario == "original-stop" || scenario == "absent-current-stop" {
				if !reflect.DeepEqual(stops[0].Request, *r.Rounds[0].Stop) {
					t.Fatal("original stop rewritten")
				}
			} else if string(stops[0].Request.Input) != `{"expected_revision":9}` {
				t.Fatal("stale Line snapshot revision used", stops)
			}
			if scenario == "absent-stale-stop" && len(saved.Rounds[0].RejectedStops) != 1 {
				t.Fatal("lost original stop history")
			}
			again, err := prepareEnvironmentStopsWith(ctx, c, store, build, query)
			require(t, err)
			if !reflect.DeepEqual(stops, again) {
				t.Fatal("restart replaced original envelope", stops, again)
			}
		})
	}
}

func TestProjectEnvironmentRejectsForeignNamespace(t *testing.T) {
	ctx := context.Background()
	store, e := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	require(t, e)
	defer store.Close()
	cli := runnerclient.Client{StateDir: t.TempDir()}
	old := cli.BuildPin()
	old.StateDir = t.TempDir()
	r := env.Record{FormatVersion: 1, ID: env.ID(), Project: "project", Revision: 1, Kind: "adaptation", Build: old, Cancelled: true, State: "stopping", Requests: []env.Request{{ID: env.ID(), Original: json.RawMessage(`{}`)}}, Rounds: []env.Round{{Number: 1, Submission: "accepted", RunID: "original-run", InputDigest: "original-input"}}}
	r.Execution = runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"access_mode":"full_access"}`)}
	r.Prepared.Identity = protocol.ExecutionIdentity(r.Execution)
	r.Prepared.DerivedExecution = r.Execution
	r.Prepared.Input.Execution = r.Execution
	require(t, store.SaveEnvironment(ctx, r, 0))
	before, e := store.Environment(ctx, r.Project, r.ID)
	require(t, e)
	if stops, e := prepareEnvironmentStops(ctx, Config{identity: "project"}, store, &cli); e == nil || len(stops) != 0 {
		t.Fatal("foreign namespace admitted", stops, e)
	}
	saved, e := store.Environment(ctx, r.Project, r.ID)
	require(t, e)
	if !reflect.DeepEqual(saved, before) {
		t.Fatal("rejection rewrote original")
	}
}
