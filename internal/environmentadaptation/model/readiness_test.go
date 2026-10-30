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
	key, err := Key(original)
	if err != nil {
		t.Fatal(err)
	}
	changed := original
	changed.Settings = json.RawMessage(`{"model":"two","access_mode":"full_access","reasoning_effort":"high","executable":"codex"}`)
	same, err := Key(changed)
	if err != nil || same != key || string(original.Settings) != before {
		t.Fatal("model grouping altered frozen execution", err)
	}
	for _, settings := range []string{`{"executable":"other","access_mode":"full_access"}`, `{"executable":"codex","access_mode":"sandbox"}`, `{"executable":"codex","access_mode":"full_access","unknown":true}`} {
		changed.Settings = json.RawMessage(settings)
		next, err := Key(changed)
		if err != nil || next == key {
			t.Fatal("other context field ignored", settings, err)
		}
	}
	for _, settings := range []string{`null`, `[]`, `{}`, `{"model":"a","model":"b"}`} {
		changed.Settings = json.RawMessage(settings)
		if _, err := Key(changed); err == nil {
			t.Fatal("invalid settings accepted", settings)
		}
	}
	if Target(original) == "" {
		t.Fatal("local target bypass")
	}
	changed = original
	changed.Target.Environment = "unexpected-local-alias"
	if _, err := Key(changed); err == nil {
		t.Fatal("local target accepted remote environment alias")
	}
	original.Adapter = "unknown"
	if _, err := Key(original); err == nil {
		t.Fatal("unsupported adapter allowed")
	}
}
func TestProjectEnvironmentReadinessRevocationAndProjection(t *testing.T) {
	execution := readinessExecution()
	key, _ := Key(execution)
	record := Record{ID: strings.Repeat("a", 32), Project: "project", Kind: "adaptation", Execution: execution, State: "ready", Prepared: Prepared{Inputs: "basis"}, Rounds: []Round{{Number: 1, Settled: true}}}
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

func TestProjectEnvironmentReadinessOrcaProjectionPreservesTransportAndArguments(t *testing.T) {
	execution := runnercontract.ExecutionInput{Adapter: "orca-terminal", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"transport":{"executable":"orca","args":["--model","unchanged"]},"agent_launch":{"executable":"agent","model":"one","reasoning_effort":"low","args":["--model","literal"]},"model":"other-field"}`)}
	before := string(execution.Settings)
	key, err := Key(execution)
	if err != nil {
		t.Fatal(err)
	}
	other := execution
	other.Settings = json.RawMessage(strings.ReplaceAll(strings.ReplaceAll(before, `"model":"one"`, `"model":"two"`), `"reasoning_effort":"low"`, `"reasoning_effort":"high"`))
	same, err := Key(other)
	if err != nil || same != key || string(execution.Settings) != before {
		t.Fatal("structured model projection differs", err)
	}
	for _, change := range []string{strings.Replace(before, "unchanged", "changed", 1), strings.Replace(before, "literal", "changed", 1), strings.Replace(before, "other-field", "changed", 1)} {
		other.Settings = json.RawMessage(change)
		next, err := Key(other)
		if err != nil || next == key {
			t.Fatal("unrelated model or argument removed", err)
		}
	}
}
func TestProjectEnvironmentReadinessStagesDoNotGrantUnsettledOrHistoricalSuccess(t *testing.T) {
	execution := readinessExecution()
	key, _ := Key(execution)
	ready := Record{ID: strings.Repeat("a", 32), Project: "project", Kind: "adaptation", Execution: execution, State: "ready", Prepared: Prepared{Inputs: "basis"}, Rounds: []Round{{Number: 1, Settled: true}}}
	current := Current{Project: "project", Key: key, Revision: 1, ActivityID: ready.ID}
	running := ready
	running.Rounds = []Round{{Number: 1}}
	running.State = "running"
	failed := ready
	failed.State = "failed"
	failed.Diagnostic = "required tool missing"
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
	key, err := Key(codex)
	if err != nil {
		t.Fatal(err)
	}
	other, err := Key(orca)
	if err != nil || key == other || Target(codex) == Target(orca) {
		t.Fatal("local adapters shared environment or occupancy", err)
	}
	record := Record{ID: strings.Repeat("a", 32), Project: "project", Kind: "adaptation", Execution: codex, State: "ready", Prepared: Prepared{Inputs: "basis"}, Rounds: []Round{{Number: 1, Settled: true}}}
	current := Current{Project: "project", Key: key, Revision: 1, ActivityID: record.ID}
	if _, err = Qualify(current, record, "basis"); err != nil {
		t.Fatal(err)
	}
	record.Execution = orca
	if _, err = Qualify(current, record, "basis"); err == nil {
		t.Fatal("Orca reused local Codex preparation")
	}
}
