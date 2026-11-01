package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/app/host"
	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	executionprocess "github.com/erranliu/SteerLine/internal/execution/process"
	"github.com/erranliu/SteerLine/internal/hostcontract"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

// Scripted public Runner/PR peer. This proves caller orchestration, not native
// Agent execution, a real PR, or the independently shipped Runner collector.
type environmentWorkflowPeer struct {
	run           runnercontract.Run
	result        []byte
	submits       int
	succeedsFirst bool
}

type environmentReserveProbe struct {
	*sqlite.SQLiteStore
	reservations int
}

func (p *environmentReserveProbe) ReserveRunnerCandidate(context.Context, taskmodel.FixedWork, runnerbridge.Candidate, bool) error {
	p.reservations++
	return nil
}

func (f *environmentWorkflowPeer) Call(_ context.Context, command string, _ []string, q *runnerclient.Request) (runnerclient.Response, error) {
	if command == "submit" {
		f.submits++
		var input runnercontract.RunInput
		if e := json.Unmarshal(q.Input, &input); e != nil {
			return runnerclient.Response{}, e
		}
		raw, e := base64.StdEncoding.DecodeString(input.ResultFile.InitialContentBase64)
		if e != nil {
			return runnerclient.Response{}, e
		}
		var report env.Report
		if e = json.Unmarshal(raw, &report); e != nil {
			return runnerclient.Response{}, e
		}
		f.run = runnercontract.Run{RunID: fmt.Sprintf("env-run-%d", f.submits), Revision: 1, Status: "failed", InputDigest: env.Digest(q.Input), Correlation: input.Correlation, Execution: runnercontract.Execution{Branch: fmt.Sprintf("env-head-%d", f.submits)}}
		f.run.Status = "completed"
		f.run.Final = &runnercontract.Final{Text: "fixture native explanation", Verified: true, NativeOutcome: "succeeded", CommandID: "original-command", SessionID: "original-session", TurnID: "original-turn"}
		report.Completed = true
		report.Ready = f.succeedsFirst
		report.Message = "project detection finished"
		if !report.Ready {
			report.Message = "fixture tool absent"
		}
		report.Details = json.RawMessage(`{"checks":[{"status":"failed"}],"format":"project-owned"}`)
		f.result, _ = json.Marshal(report)
	}
	if command == "stop" {
		f.run.StopStatus = "stopped"
		if f.run.Status != "completed" && f.run.Status != "failed" {
			f.run.Status = "stopped"
		}
		f.run.Revision++
	}
	return runnerclient.Response{}, nil
}
func (f *environmentWorkflowPeer) Inspect(context.Context, string, string, bool) (runnerclient.Inspection, error) {
	copy := f.run
	return runnerclient.Inspection{Run: &copy}, nil
}
func (f *environmentWorkflowPeer) file(contract string, raw []byte, version uint64) runnercontract.ProgressFile {
	encoded := base64.StdEncoding.EncodeToString(raw)
	now := time.Now().UTC()
	status := "observing"
	if f.run.StopStatus == "stopped" {
		status = "closed"
	}
	return runnercontract.ProgressFile{Contract: contract, RunID: f.run.RunID, Version: version, ObservedAt: &now, Encoding: "base64", ContentBase64: &encoded, SizeBytes: len(raw), SHA256: env.Digest(raw), CollectionStatus: status}
}
func (f *environmentWorkflowPeer) ProgressFile(context.Context, string) (runnercontract.ProgressFile, error) {
	return f.file(runnercontract.ProgressContract, []byte(`{"test.one":true,"pr":true}`), 17), nil
}
func (f *environmentWorkflowPeer) ResultFile(context.Context, string) (runnercontract.ResultFile, error) {
	if f.run.StopStatus != "stopped" {
		return runnercontract.ResultFile{Contract: runnercontract.ResultContract, RunID: f.run.RunID, Encoding: "base64", CollectionStatus: "pending"}, nil
	}
	return f.file(runnercontract.ResultContract, f.result, 1), nil
}

func TestProjectEnvironmentWorkflowPublicCommandsAndSingleInvocation(t *testing.T) {
	for _, pass := range []bool{false, true} {
		t.Run(fmt.Sprint(pass), func(t *testing.T) {
			port, c, store := environmentFixture(t)
			fixtureGit(t, c.Repository, "push", "origin", "HEAD:refs/heads/environment-validation")
			peer := &environmentWorkflowPeer{succeedsFirst: pass}
			port.service.Runner = peer
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			taskService := runnerTaskService(c, store, runnerclient.Client{})
			actor, e := taskrunner.NewActor(taskrunner.ActorOptions{Tasks: taskService, Store: store, Host: host.NewServiceHost(host.Config{RunDir: c.RunDir, ProcessFacts: executionprocess.NewServiceFacts()}, "off", io.Discard, nil), Repository: hostcontract.Repository{Identity: c.Identity()}, Mode: "manual", Heartbeat: time.Second, Control: func(context.Context, api.ControlRequest) (api.ControlReceipt, error) {
				return api.ControlReceipt{}, nil
			}})
			require(t, e)
			require(t, actor.BeginActor(ctx))
			done := make(chan struct{})
			go func() {
				defer close(done)
				for {
					select {
					case call := <-actor.Signals():
						_ = actor.HandleSignal(call)
					case <-ctx.Done():
						return
					}
				}
			}()
			defer func() { cancel(); <-done }()
			runtime := &runtimeProjectPort{actor: actor, config: c, store: store, instance: actor.InstanceID()}
			key, _ := env.Key(c.RunnerExecution.Profiles[1].Execution)
			payload, _ := json.Marshal(env.Start{EnvironmentKey: key, Profile: "remote", TimeoutSeconds: 60})
			request := httpcontract.CommandEnvelope{RequestID: env.ID(), ProjectID: httpcontract.ProjectID(c.Identity()), InstanceID: actor.InstanceID(), Action: "environment_start", TargetID: httpcontract.ProjectID(c.Identity()), ExpectedVersion: 1, Payload: payload}
			for _, invalid := range []string{`{"profile_id":"remote","round_timeout_seconds":60}`, `{"profile_id":"remote","round_timeout_seconds":60,"expected_current_revision":null}`} {
				bad := request
				bad.RequestID = env.ID()
				bad.Payload = json.RawMessage(invalid)
				receipt, err := runtime.Command(ctx, bad)
				require(t, err)
				if receipt.Status != "rejected" || peer.submits != 0 {
					t.Fatal("missing current revision reached Runner", receipt)
				}
				rows, err := store.Environments(ctx, c.Identity())
				require(t, err)
				if len(rows) != 0 {
					t.Fatal("invalid start selected activity")
				}
			}
			out, e := runtime.Command(ctx, request)
			require(t, e)
			if out.Status != "applied" {
				t.Fatal(out)
			}
			again, e := runtime.Command(ctx, request)
			require(t, e)
			if again.Status != "applied" {
				t.Fatal(again)
			}
			for i := 0; i < 6; i++ {
				require(t, port.service.Observe(ctx))
			}
			saved, e := store.Environment(ctx, c.Identity(), request.RequestID)
			require(t, e)
			wantRounds := 1
			if len(saved.Rounds) != 1 || peer.submits != 1 || (saved.Status() == "ready") != pass || saved.Rounds[0].Report.Ready != pass {
				t.Fatal("single invocation result consumption failed", saved.State, peer.submits)
			}
			value, e := runtime.Read(ctx, "/environment/"+saved.ID, nil)
			require(t, e)
			raw, _ := json.Marshal(value)
			if strings.Contains(string(raw), "agent_launch") || strings.Contains(string(raw), "initial_content_base64") || !strings.Contains(string(raw), "\"report\"") || !strings.Contains(string(raw), "\"result\"") || !strings.Contains(string(raw), "fixture native explanation") {
				t.Fatal("public view leaked frozen input or lost report")
			}
			if _, _, e = port.admission(ctx, "remote", "", false); (e == nil) != pass {
				t.Fatal("environment readiness differs from complete sealed report", e)
			}
			probe := &environmentReserveProbe{SQLiteStore: store}
			gate := runnerTaskService(c, probe, runnerclient.Client{})
			profile, e := c.RunnerExecution.Profile("remote", "")
			require(t, e)
			input, _ := json.Marshal(runnercontract.RunInput{Execution: profile.Execution})
			err := gate.ReserveExecution(ctx, taskmodel.FixedWork{}, runnerbridge.Candidate{ProfileID: "remote", Submit: runnerclient.Request{Input: input}}, false)
			if (err == nil) != pass || (probe.reservations == 1) != pass {
				t.Fatal("ready/blocked environment crossed the wrong reservation boundary", err)
			}
			request.Payload = json.RawMessage(`{"profile_id":"remote","round_timeout_seconds":90}`)
			if _, e = runtime.Command(ctx, request); e == nil {
				t.Fatal("same request ID changed original input")
			}
			receipt, e := runtime.Read(ctx, "/commands/"+saved.ID, nil)
			require(t, e)
			if receipt == nil {
				t.Fatal("original request receipt missing")
			}
			if peer.submits != wantRounds {
				t.Fatal("receipt query/repeat command resubmitted")
			}
		})
	}
}
