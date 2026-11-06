package model

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func readinessExecution() runnercontract.ExecutionInput {
	return runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"executable":"codex","model":"one","reasoning_effort":"medium","access_mode":"full_access"}`)}
}
func TestProjectEnvironmentReadinessContextKeepsOriginalAndGroupsModels(t *testing.T) {
	original := readinessExecution()
	before := string(original.Settings)
	key, err := Key(original, fixtureExecutionIdentity(original))
	if err != nil {
		t.Fatal(err)
	}
	changed := original
	changed.Settings = json.RawMessage(`{"model":"two","access_mode":"full_access","reasoning_effort":"high","executable":"codex"}`)
	same, err := Key(changed, fixtureExecutionIdentity(changed))
	if err != nil || same != key || string(original.Settings) != before {
		t.Fatal("model grouping altered frozen execution", err)
	}
	for _, settings := range []string{`{"executable":"other","access_mode":"full_access"}`, `{"executable":"codex","access_mode":"sandbox"}`, `{"executable":"codex","access_mode":"full_access","unknown":true}`} {
		changed.Settings = json.RawMessage(settings)
		next, err := Key(changed, fixtureExecutionIdentity(changed))
		if err != nil || next == key {
			t.Fatal("other context field ignored", settings, err)
		}
	}
	for _, settings := range []string{`null`, `[]`, `{}`, `{"model":"a","model":"b"}`} {
		changed.Settings = json.RawMessage(settings)
		if _, err := Key(changed, fixtureExecutionIdentity(changed)); err == nil {
			t.Fatal("invalid settings accepted", settings)
		}
	}
	changed = original
	changed.Target.Environment = "unexpected-local-alias"
	if _, err := Key(changed, fixtureExecutionIdentity(changed)); err == nil {
		t.Fatal("local target accepted remote environment alias")
	}
	original.Adapter = "unknown"
	if _, err := Key(original, fixtureExecutionIdentity(original)); err == nil {
		t.Fatal("unsupported adapter allowed")
	}
}
func TestProjectEnvironmentReadinessRevocationAndProjection(t *testing.T) {
	execution := readinessExecution()
	key, _ := Key(execution, fixtureExecutionIdentity(execution))
	record := Record{ID: strings.Repeat("a", 32), Project: "project", Kind: "adaptation", Execution: execution, State: "ready", Prepared: Prepared{Identity: fixtureExecutionIdentity(execution), DerivedExecution: execution, Inputs: "basis"}, Rounds: []Round{{Number: 1, Settled: true}}}
	current := Current{Project: "project", Key: key, Revision: 1, ActivityID: record.ID}
	if _, err := Qualify(current, record, "basis"); err != nil {
		t.Fatal(err)
	}
	revoked := current
	revoked.Invalidated = true
	revoked.Reason = "machine reset"
	if _, err := Qualify(revoked, record, "basis"); err == nil {
		t.Fatal("late ready undid revocation")
	}
	if status, stage, _ := View(revoked, record, "basis", nil); status != "not_ready" || stage != "stale" {
		t.Fatal(status, stage)
	}
	if _, err := Qualify(current, record, "changed"); err == nil {
		t.Fatal("stale basis accepted")
	}
	current.Project = "other"
	if _, err := Qualify(current, record, "basis"); err == nil {
		t.Fatal("foreign project accepted")
	}
	if status, stage, _ := View(Current{}, Record{}, "", nil); status != "not_ready" || stage != "awaiting_user" {
		t.Fatal(status, stage)
	}
}

func TestProjectEnvironmentReadinessConsumesOpaqueRunnerIdentity(t *testing.T) {
	source := runnercontract.ExecutionInput{Adapter: "orca-terminal", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"opaque_native_setting":"saved"}`)}
	identity := runnercontract.ExecutionIdentity{Contract: runnercontract.ExecutionIdentityContract, SourceExecutionDigest: runnercontract.ConfigurationValueDigest(source), Context: runnercontract.ExecutionInput{Adapter: source.Adapter, Target: source.Target, Settings: json.RawMessage(`{"opaque_native_setting":"projected"}`)}}
	key, err := Key(source, identity)
	if err != nil || key != runnercontract.ConfigurationValueDigest(identity.Context) {
		t.Fatal("caller reinterpreted Runner's opaque projection", err)
	}
	changed := source
	changed.Settings = json.RawMessage(`{"opaque_native_setting":"other"}`)
	if _, err = Key(changed, identity); err == nil {
		t.Fatal("identity accepted a different source")
	}
	identity.Contract = "other"
	if _, err = Key(source, identity); err == nil {
		t.Fatal("foreign identity accepted")
	}
	identity.Contract = runnercontract.ExecutionIdentityContract
	identity.Context.Settings = json.RawMessage(`{"opaque_native_setting":"one","opaque_native_setting":"two"}`)
	if _, err = Key(source, identity); err == nil {
		t.Fatal("ambiguous context returned an empty successful key")
	}
}

func TestProjectEnvironmentReadinessStagesDoNotGrantUnsettledOrHistoricalSuccess(t *testing.T) {
	execution := readinessExecution()
	key, _ := Key(execution, fixtureExecutionIdentity(execution))
	ready := Record{ID: strings.Repeat("a", 32), Project: "project", Kind: "adaptation", Execution: execution, State: "ready", Prepared: Prepared{Identity: fixtureExecutionIdentity(execution), DerivedExecution: execution, Inputs: "basis"}, Rounds: []Round{{Number: 1, Settled: true}}}
	current := Current{Project: "project", Key: key, Revision: 1, ActivityID: ready.ID}
	running := ready
	running.Rounds = []Round{{Number: 1}}
	running.State = "running"
	running.Diagnostic = "receipt query: context deadline exceeded"
	timingOut := running
	timingOut.Rounds = []Round{{Number: 1, TimedOut: true}}
	failed := ready
	failed.State = "failed"
	failed.Diagnostic = "required tool missing"
	timedOut := failed
	timedOut.Rounds = []Round{{Number: 1, TimedOut: true, Settled: true}}
	revoked := current
	revoked.Invalidated = true
	revoked.Reason = "machine reset"
	for _, test := range []struct {
		name     string
		current  Current
		record   Record
		basis    string
		material error
		stage    string
	}{
		{"missing-material", Current{}, Record{}, "", errors.New("missing document"), "unprepared"},
		{"awaiting-user", Current{}, Record{}, "", nil, "awaiting_user"},
		{"running", current, running, "basis", nil, "verifying"},
		{"timeout-cleanup", current, timingOut, "basis", nil, "timed_out"},
		{"settled-timeout", current, timedOut, "basis", nil, "timed_out"},
		{"failed", current, failed, "basis", nil, "blocked"},
		{"basis-changed", current, ready, "new", nil, "stale"},
		{"revoked-with-unsettled-writer", revoked, running, "basis", nil, "stale"},
		{"current-not-history", current, Record{}, "basis", nil, "blocked"},
		{"ready", current, ready, "basis", nil, "ready"},
	} {
		t.Run(test.name, func(t *testing.T) {
			status, stage, _ := View(test.current, test.record, test.basis, test.material)

			if stage != test.stage || (status == "ready") != (stage == "ready") {
				t.Fatal(status, stage)
			}
		})
	}
}

func TestProjectEnvironmentReadinessLocalAdaptersHaveSeparateQualification(t *testing.T) {
	codex := readinessExecution()
	orca := runnercontract.ExecutionInput{Adapter: "orca-terminal", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"transport":{"executable":"orca"},"agent_launch":{"executable":"codex"},"access_mode":"full_access"}`)}
	key, err := Key(codex, fixtureExecutionIdentity(codex))
	if err != nil {
		t.Fatal(err)
	}
	other, err := Key(orca, fixtureExecutionIdentity(orca))
	if err != nil || key == other {
		t.Fatal("local adapters shared environment or occupancy", err)
	}
	record := Record{ID: strings.Repeat("a", 32), Project: "project", Kind: "adaptation", Execution: codex, State: "ready", Prepared: Prepared{Identity: fixtureExecutionIdentity(codex), DerivedExecution: codex, Inputs: "basis"}, Rounds: []Round{{Number: 1, Settled: true}}}
	current := Current{Project: "project", Key: key, Revision: 1, ActivityID: record.ID}
	if _, err = Qualify(current, record, "basis"); err != nil {
		t.Fatal(err)
	}
	record.Execution = orca
	if _, err = Qualify(current, record, "basis"); err == nil {
		t.Fatal("Orca reused local Codex preparation")
	}
}

// Synthetic facts; this pure model never implements native configuration rules.
func fixtureExecutionIdentity(execution runnercontract.ExecutionInput) runnercontract.ExecutionIdentity {
	context := execution
	var settings map[string]json.RawMessage
	_ = json.Unmarshal(execution.Settings, &settings)
	if execution.Adapter == "codex-app-server" {
		delete(settings, "model")
		delete(settings, "reasoning_effort")
	} else if raw, ok := settings["agent_launch"]; ok {
		var launch map[string]json.RawMessage
		_ = json.Unmarshal(raw, &launch)
		delete(launch, "model")
		delete(launch, "reasoning_effort")
		if len(launch) == 0 {
			delete(settings, "agent_launch")
		} else {
			settings["agent_launch"], _ = json.Marshal(launch)
		}
	}
	context.Settings, _ = json.Marshal(settings)
	return runnercontract.ExecutionIdentity{Contract: runnercontract.ExecutionIdentityContract, SourceExecutionDigest: runnercontract.ConfigurationValueDigest(execution), Context: context}
}
