package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/developerconfig"
	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestDeveloperConfigurationOptions(t *testing.T) {
	for _, skip := range []bool{false, true} {
		p := &environmentPort{config: Config{RunnerExecution: RunnerExecutionConfig{Developer: developerconfig.Config{SkipEnvironmentReadiness: skip}}}}
		options := p.configurationOptions()
		if options.ReadinessRequired != !(skip && developerconfig.Available) {
			t.Fatal("configuration-only response lost the frozen admission policy", options)
		}
	}
}

func TestDeveloperConfigurationLoad(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	c.RunnerExecution.Developer.SkipEnvironmentReadiness = true
	raw, err := json.Marshal(c.RunnerExecution)
	require(t, err)
	require(t, os.WriteFile(GlobalExecutionConfigPath(), raw, 0600))
	loaded, err := LoadConfig(c.ConfigPath)
	if !developerconfig.Available {
		if err == nil || !strings.Contains(err.Error(), "developer configuration is disabled") {
			t.Fatal("release loaded a developer override", err)
		}
		return
	}
	require(t, err)
	if !developerconfig.Resolve(loaded.RunnerExecution.Developer).SkipEnvironmentReadiness() {
		t.Fatal("saved global switch was not loaded")
	}
	c.RunnerExecution.Developer.SkipEnvironmentReadiness = false
	raw, err = json.Marshal(c.RunnerExecution)
	require(t, err)
	require(t, os.WriteFile(GlobalExecutionConfigPath(), raw, 0600))
	if !developerconfig.Resolve(loaded.RunnerExecution.Developer).SkipEnvironmentReadiness() {
		t.Fatal("running configuration changed without restart")
	}
	reloaded, err := LoadConfig(c.ConfigPath)
	require(t, err)
	if developerconfig.Resolve(reloaded.RunnerExecution.Developer).SkipEnvironmentReadiness() {
		t.Fatal("restart did not restore default gate")
	}
}

func TestDeveloperReadinessService(t *testing.T) {
	p, c, store := environmentFixture(t)
	ctx := context.Background()
	c.RunnerExecution.DefaultProfile = "remote"
	c.RunnerExecution.Developer.SkipEnvironmentReadiness = true
	probe := &environmentReserveProbe{SQLiteStore: store}
	service := runnerTaskService(c, probe, runnerclient.Client{StateDir: filepath.Join(t.TempDir(), "runner")})
	if err := service.CheckExecution(ctx, "remote", true); (err == nil) != developerconfig.Available {
		t.Fatal("service ignored build policy", err)
	}
	if !developerconfig.Available {
		return
	}
	// A running service keeps its original policy if the source value changes.
	c.RunnerExecution.Developer.SkipEnvironmentReadiness = false
	require(t, service.CheckExecution(ctx, "remote", false))
	strict := runnerTaskService(c, probe, runnerclient.Client{})
	if err := strict.CheckExecution(ctx, "remote", false); err == nil {
		t.Fatal("default did not restore gate")
	}
	service.Tasks.Reader = readinessTasks{c.Identity()}
	selected := service.Profiles["remote"]
	selected.UnavailableReason = ""
	service.Profiles["remote"] = selected
	preparing := errors.New("reached material preparation")
	service.PrepareExecution = func(context.Context, taskmodel.FixedWork, bool) (taskmodel.FixedWork, string, error) {
		return taskmodel.FixedWork{}, "", preparing
	}
	if _, err := service.Start(ctx, taskrunnerStartForReadiness()); !errors.Is(err, preparing) {
		t.Fatal("manual start still blocked", err)
	}
	if err := service.Dispatch(ctx); !errors.Is(err, preparing) {
		t.Fatal("automatic dispatch still blocked", err)
	}
	selected.UnavailableReason = "unsupported execution"
	service.Profiles["remote"] = selected
	if _, err := service.Start(ctx, taskrunnerStartForReadiness()); err == nil || !strings.Contains(err.Error(), "unsupported execution") {
		t.Fatal("skip bypassed profile availability", err)
	}
	profile, err := c.RunnerExecution.Profile("remote", "")
	require(t, err)
	raw, _ := json.Marshal(runnercontract.RunInput{Execution: profile.Execution})
	candidate := runnerbridge.Candidate{ProfileID: "remote", Submit: runnerclient.Request{Input: raw}}
	require(t, service.ReserveExecution(ctx, taskmodel.FixedWork{}, candidate, true))
	if probe.reservations != 1 {
		t.Fatal("extra candidate did not reach reservation")
	}
	// Keep the environment report honest even while the gate is disabled.
	p.config.RunnerExecution.Developer.SkipEnvironmentReadiness = true
	value, err := p.readinessOptions(ctx)
	require(t, err)
	options := value.(environmentOptions)
	if options.ReadinessRequired {
		t.Fatal(options)
	}
	if p.configurationOptions().ReadinessRequired != options.ReadinessRequired {
		t.Fatal("configuration and qualification responses disagree on admission policy")
	}
	for _, item := range options.Environments {
		if item.Status == "ready" {
			t.Fatal("developer override fabricated readiness")
		}
	}
	// In-flight adaptation still owns its execution target.
	prepared, err := p.prepare(ctx, "remote")
	require(t, err)
	record := env.Record{ID: env.ID(), Project: c.Identity(), Revision: 1, Kind: "adaptation", Profile: "remote", Execution: prepared.Input.Execution, Prepared: prepared,
		State: "running", CreatedAt: time.Now().UTC(), Requests: []env.Request{{ID: env.ID(), Original: json.RawMessage(`{}`)}}, Rounds: []env.Round{{Number: 1}}}
	require(t, store.StartEnvironment(ctx, record, 0))
	if err := service.ReserveExecution(ctx, taskmodel.FixedWork{}, candidate, false); err == nil || !strings.Contains(err.Error(), "未核实的环境适配") {
		t.Fatal("skip bypassed target occupancy", err)
	}
}
