package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
)

func TestStartupCancellationOutboxIsDurableBeforeRunnerStart(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	prepareStoppedRunRepository(t, c)
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-fixture")
	require(t, os.MkdirAll(EtcDir(), 0700))
	require(t, os.WriteFile(filepath.Join(EtcDir(), "one.json"), projectConfigJSON(t, c), 0600))
	d, r := startPresetDirectory(t)
	candidate := seedStoppedPublicRun(t, r, c)
	require(t, d.Close())
	ctx := context.Background()
	store, err := OpenNativeTaskCommandStore(ctx, &c)
	require(t, err)
	candidate, err = store.Candidate(ctx, c.Identity(), candidate.ID)
	require(t, err)
	expected := candidate.Revision
	candidate.Revision++
	candidate.StopRequested = true
	candidate.InputsRevoked = false
	candidate.Stop = nil
	require(t, store.SaveCandidate(ctx, candidate, expected, nil))
	require(t, store.Close())
	cli := r.runner.client
	cli.InstanceID = ""
	view := runnercontract.Run{RunID: candidate.RunID, InputDigest: candidate.InputDigest, Revision: 7}
	writeStartupInspection(t, cli, view, candidate.Submit.RequestID)
	stops, err := prepareProjectRunnerStops(ctx, &c, &cli)
	require(t, err)
	if len(stops) != 1 || stops[0].RunID != candidate.RunID || string(stops[0].Request.Input) != `{"expected_revision":7}` {
		t.Fatal("startup stop did not bind original Run/revision", stops)
	}
	again, err := prepareProjectRunnerStops(ctx, &c, &cli)
	require(t, err)
	if !bytes.Equal(mustJSON(stops), mustJSON(again)) {
		t.Fatal("restart replaced original stop envelope")
	}
	reader, err := OpenNativeTaskReader(ctx, &c)
	require(t, err)
	defer reader.Close()
	saved, err := reader.Candidate(ctx, c.Identity(), candidate.ID)
	require(t, err)
	if saved.Stop == nil || saved.Stop.State != "unknown" || saved.Stop.Request.RequestID != stops[0].Request.RequestID || saved.RunnerBuild != candidate.RunnerBuild || saved.RunID != candidate.RunID || saved.Submit.RequestID != candidate.Submit.RequestID {
		t.Fatal("startup outbox was not durable or rewrote frozen facts", saved)
	}
	info, err := cli.ServiceStatus(ctx)
	require(t, err)
	if info.Status != "not_running" {
		t.Fatal("outbox preparation started Runner")
	}
}

type startupCandidateStore struct{ value runnerstate.Candidate }

func (s *startupCandidateStore) Candidate(context.Context, string, string) (runnerstate.Candidate, error) {
	return s.value, nil
}
func (s *startupCandidateStore) SaveCandidate(_ context.Context, c runnerstate.Candidate, expected uint64, _ []runnercontract.Event) error {
	if expected != s.value.Revision {
		return errors.New("stale candidate")
	}
	s.value = c
	return nil
}

func TestStartupCancelledCandidateColdAbsenceNeverResubmits(t *testing.T) {
	for _, cold := range []bool{true, false} {
		t.Run(map[bool]string{true: "cold", false: "online"}[cold], func(t *testing.T) {
			cli := startupFixtureCLI(t)
			if !cold {
				cli.ResourceDigest = strings.Repeat("a", 64)
				require(t, os.WriteFile(filepath.Join(cli.StateDir, "idle-fixture.json"), mustJSON(runnerclient.ServiceInfo{Protocol: runnerclient.Protocol, StateDir: cli.StateDir, Status: "ready", InstanceID: "original", Version: cli.BuildVersion, ResourceDigest: cli.ResourceDigest, ExecutableSHA256: cli.SHA256}), 0600))
			}
			require(t, os.WriteFile(filepath.Join(cli.StateDir, "idle-run-fixture.json"), []byte(`{"fixture_error":"request_not_found"}`), 0600))
			c := runnerstate.Candidate{ID: "candidate", Project: "project", Revision: 1, RunnerBuild: cli.BuildPin(), StateDir: cli.StateDir, SubmitState: "unknown", StopRequested: true, Submit: runnercontract.Request{Protocol: runnerclient.Protocol, RequestID: "original-submit"}}
			s := &startupCandidateStore{value: c}
			stops, err := prepareCandidateStops(context.Background(), []runnerstate.Candidate{c}, s, cli)
			if cold {
				require(t, err)
				if !s.value.SubmitObserveOnly || s.value.SubmitState != "unknown" {
					t.Fatal("cold missing submit regained dispatch authority", s.value)
				}
			} else if err == nil || s.value.Revision != c.Revision {
				t.Fatal("online missing request was treated as final absence", err)
			}
			if len(stops) != 0 || s.value.RunID != "" || s.value.Submit.RequestID != c.Submit.RequestID {
				t.Fatal("missing submit created Run/stop or changed original identity", stops, s.value)
			}
			if _, err := os.Stat(filepath.Join(cli.StateDir, "business-dispatch.log")); !os.IsNotExist(err) {
				t.Fatal("pre-start inspection dispatched business")
			}
		})
	}
}
