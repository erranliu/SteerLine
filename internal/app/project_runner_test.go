package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunnerSettlementReadCanRecoverWithoutRepeatingStop(t *testing.T) {
	stops, reads := 0, 0
	r := &applicationRunner{attempted: true, stop: func(context.Context) error { stops++; return nil }, settle: func(context.Context) error {
		reads++
		if reads == 1 {
			return context.DeadlineExceeded
		}
		return nil
	}}
	if err := r.shutdown(context.Background()); err != nil || r.ResourceSettlementError() == nil {
		t.Fatal("daemon exit replaced final resource proof", err)
	}
	if err := r.shutdown(context.Background()); err != nil || r.ResourceSettlementError() != nil || stops != 1 || reads != 2 {
		t.Fatal(err, r.ResourceSettlementError(), stops, reads)
	}
}
func TestRunnerShutdownRetriesOnlyUnconfirmedOriginalStop(t *testing.T) {
	stops, reads := 0, 0
	r := &applicationRunner{attempted: true, stop: func(context.Context) error {
		stops++
		if stops == 1 {
			return context.DeadlineExceeded
		}
		return nil
	}, settle: func(context.Context) error { reads++; return nil }}
	if r.ResourceSettlementError() == nil {
		t.Fatal("unattempted shutdown guessed successful")
	}
	if r.shutdown(context.Background()) == nil || r.ResourceSettlementError() == nil || reads != 0 {
		t.Fatal("unknown stop became settled")
	}
	if err := r.shutdown(context.Background()); err != nil || r.ResourceSettlementError() != nil {
		t.Fatal(err, r.ResourceSettlementError())
	}
	if err := r.shutdown(context.Background()); err != nil || stops != 2 || reads != 1 {
		t.Fatal("confirmed stop repeated", err, stops, reads)
	}
}
func TestSharedRunnerProjectValidationKeepsService(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-fixture")
	var shared applicationRunner
	require(t, shared.start(context.Background(), c.RunnerExecution, c.runnerHome(), 5*time.Second))
	defer shared.shutdown(context.Background())
	c.PR.Kind = "unsupported-fixture-provider"
	if shared.validateProject(c) == nil {
		t.Fatal("invalid project publication accepted")
	}
	info, err := shared.client.ServiceStatus(context.Background())
	require(t, err)
	if info.InstanceID != shared.info.InstanceID || info.Status != "ready" {
		t.Fatal("project validation stopped shared service", info)
	}
}

func TestRunnerUnknownStartKeepsEvidenceWithoutStoppingUntrustedTarget(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-fixture")
	var original applicationRunner
	require(t, original.start(context.Background(), c.RunnerExecution, c.runnerHome(), 5*time.Second))
	defer original.shutdown(context.Background())
	// This is the idle public CLI fixture's data, not Runner private state.
	path := filepath.Join(original.client.StateDir, "idle-fixture.json")
	raw, err := os.ReadFile(path)
	require(t, err)
	var observed map[string]any
	require(t, json.Unmarshal(raw, &observed))
	observed["version"] = "untrusted"
	raw, err = json.Marshal(observed)
	require(t, err)
	require(t, os.WriteFile(path, raw, 0600))
	var rejected applicationRunner
	if rejected.start(context.Background(), c.RunnerExecution, c.runnerHome(), 5*time.Second) == nil {
		t.Fatal("untrusted release accepted")
	}
	if !rejected.attempted || rejected.info.InstanceID != original.info.InstanceID || rejected.client.StateDir != original.client.StateDir || rejected.stop != nil {
		t.Fatal("partial evidence discarded or untrusted target adopted", rejected.info)
	}
	if rejected.shutdown(context.Background()) == nil || rejected.ResourceSettlementError() == nil {
		t.Fatal("unknown start reported settled")
	}
	after, err := os.ReadFile(path)
	require(t, err)
	if !bytes.Equal(raw, after) {
		t.Fatal("unknown startup issued a stop or rewrote original facts")
	}
}
