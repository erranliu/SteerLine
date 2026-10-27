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
	env "github.com/erranliu/SteerLine/internal/environmentadaptation"
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
	run     runnercontract.Run
	result  []byte
	submits int
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
		f.run.Final = &runnercontract.Final{Text: "fixture native explanation", Verified: true}
		report.Completed = true
		report.Checks = []env.Check{{ID: "tool", Status: "passed", Message: "required version available"}}
		report.L2 = []env.Item{{ID: "one", Status: "failed", Message: "fixture tool absent"}}
		report.Message = "fixture prepare failed"
		if f.submits == 2 {
			f.run.Status = "pr_ready"
			report.Message = ""
			report.L2[0].Status = "passed"
			report.L2[0].Message = ""
			report.PR = env.PR{URL: "https://example.invalid/pulls/2", Base: input.Publication.BaseBranch, Head: f.run.Execution.Branch, ChecksPassed: true, ReviewsPassed: true, Merged: true, MergeCommit: strings.Repeat("f", 40), CleanupCompleted: true}
			f.run.PR = &runnercontract.PRFact{URL: report.PR.URL, HeadBranch: report.PR.Head, BaseBranch: report.PR.Base, Merged: true}
		}
		f.result, _ = json.Marshal(report)
	}
	if command == "stop" {
		f.run.StopStatus = "stopped"
		f.run.Status = "stopped"
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

func TestProjectEnvironmentWorkflowPublicCommandsAndRetry(t *testing.T) {
	port, c, store := environmentFixture(t)
	fixtureGit(t, c.Repository, "push", "origin", "HEAD:refs/heads/environment-validation")
	peer := &environmentWorkflowPeer{}
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
	payload, _ := json.Marshal(env.Start{Profile: "remote", TimeoutSeconds: 60})
	request := httpcontract.CommandEnvelope{RequestID: env.ID(), ProjectID: httpcontract.ProjectID(c.Identity()), InstanceID: actor.InstanceID(), Action: "environment_start", TargetID: httpcontract.ProjectID(c.Identity()), ExpectedVersion: 1, Payload: payload}
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
	if saved.Status() != "ready" || len(saved.Rounds) != 2 || peer.submits != 2 || saved.Rounds[0].Report.L2[0].Status != "failed" {
		t.Fatal("caller retry/settlement failed", saved.State, peer.submits)
	}
	value, e := runtime.Read(ctx, "/environment/"+saved.ID, nil)
	require(t, e)
	raw, _ := json.Marshal(value)
	if strings.Contains(string(raw), "agent_launch") || strings.Contains(string(raw), "initial_content_base64") || !strings.Contains(string(raw), "\"report\"") || !strings.Contains(string(raw), "\"result\"") || !strings.Contains(string(raw), "fixture native explanation") {
		t.Fatal("public view leaked frozen input or lost report")
	}
	if _, e = port.ready(ctx, "remote"); e != nil {
		t.Fatal("passing report did not admit new remote work", e)
	}
	probe := &environmentReserveProbe{SQLiteStore: store}
	gate := runnerTaskService(c, probe, runnerclient.Client{})
	profile, e := c.RunnerExecution.Profile("remote", "")
	require(t, e)
	input, _ := json.Marshal(runnercontract.RunInput{Execution: profile.Execution})
	require(t, gate.ReserveExecution(ctx, taskmodel.FixedWork{}, runnerbridge.Candidate{ProfileID: "remote", Submit: runnerclient.Request{Input: input}}, false))
	if probe.reservations != 1 {
		t.Fatal("ready remote did not reach the common reservation boundary")
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
	if peer.submits != 2 {
		t.Fatal("receipt query/repeat command resubmitted")
	}
}

func TestProjectEnvironmentGenerationExportUsesMachineMetadata(t *testing.T) {
	document := "PowerShell verify-environment.ps1 is not selected.\n```steerline-environment\n{\"version\":1,\"script\":\"scripts/verify-environment.sh\",\"interpreter\":\"bash\",\"validation_base\":\"validation\",\"dependencies\":[]}\n```"
	raw, e := json.Marshal(publicEnvironment(env.Record{Kind: "generation", State: "generated", Generation: &env.Generated{Document: document, Script: "#!/usr/bin/env bash\n", Gaps: []string{}}}))
	require(t, e)
	var result struct {
		Generated struct {
			ScriptPath string `json:"script_path"`
			Document   string `json:"document"`
			Script     string `json:"script"`
		} `json:"generated"`
	}
	require(t, json.Unmarshal(raw, &result))
	if result.Generated.ScriptPath != "scripts/verify-environment.sh" || result.Generated.Document != document || result.Generated.Script == "" {
		t.Fatal("export inferred a path from prose or lost paired content")
	}
}
