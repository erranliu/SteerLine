package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	roadmapservice "github.com/erranliu/SteerLine/internal/roadmap"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

type contentStopRace struct {
	run      runnercontract.Run
	requests []runnercontract.Request
}

func (f *contentStopRace) Submit(context.Context, runnercontract.Request) (runnerclient.Receipt, error) {
	return runnerclient.Receipt{}, errors.New("close must not submit")
}
func (f *contentStopRace) Inspect(context.Context, string, string, bool) (runnerclient.Inspection, error) {
	r := f.run
	return runnerclient.Inspection{Run: &r}, nil
}
func (f *contentStopRace) Call(_ context.Context, command string, args []string, r *runnercontract.Request) (runnerclient.Response, error) {
	if command != "stop" || len(args) != 2 || args[1] != f.run.RunID {
		return runnerclient.Response{}, errors.New("wrong stop target")
	}
	f.requests = append(f.requests, *r)
	if len(f.requests) == 1 {
		f.run.Revision++
		return runnerclient.Response{}, &runnerclient.Failure{Code: "stale_revision"}
	}
	var input struct {
		Revision uint64 `json:"expected_revision"`
	}
	_ = json.Unmarshal(r.Input, &input)
	if input.Revision != f.run.Revision || r.RequestID == f.requests[0].RequestID {
		return runnerclient.Response{}, errors.New("stop did not reobserve")
	}
	f.run.StopStatus = "stopped"
	body, _ := json.Marshal(runnerclient.Receipt{RunID: f.run.RunID, CommandID: "stop", ReceiptStatus: "accepted"})
	return runnerclient.Response{Data: body}, nil
}

func TestContentCloseReconcilesStopRaceThroughSQLite(t *testing.T) {
	for _, owner := range []string{"roadmap", "test-generation"} {
		t.Run(owner, func(t *testing.T) {
			s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
			require(t, err)
			defer s.Close()
			id := strings.Repeat("a", 32)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			f := &contentStopRace{run: runnercontract.Run{RunID: "original-run", InputDigest: "input", Revision: 4, Workflow: runnercontract.Workflow{ID: "generate", Version: 1}, Correlation: map[string]string{"steerline_project": "p", "steerline_generation": id, "steerline_roadmap_parse": id}}}
			request := runnercontract.Request{RequestID: "original-submit", Input: json.RawMessage(`{}`)}
			if owner == "roadmap" {
				r := roadmap.Record{ID: id, RequestID: id, Project: "p", Revision: 1, State: "accepted", Submission: "started", Request: request, RunID: f.run.RunID, InputDigest: f.run.InputDigest}
				require(t, s.SaveRoadmap(ctx, r, 0))
				service := &roadmapservice.Service{Project: "p", Store: s, Runner: f}
				require(t, service.Close(ctx))
				got, err := s.Roadmap(ctx, "p", id)
				require(t, err)
				if !got.Settled() {
					t.Fatal("SQLite close did not settle", got)
				}
			} else {
				r := testmanagement.Generation{ID: id, Project: "p", Revision: 1, State: "accepted", Submission: "started", Request: request, RunID: f.run.RunID, InputDigest: f.run.InputDigest}
				require(t, s.SaveGeneration(ctx, r, 0))
				service := &testmanagement.GenerationService{Project: "p", Store: s, Runner: f}
				require(t, service.Close(ctx))
				got, err := s.Generation(ctx, "p", id)
				require(t, err)
				if got.StopState != "accepted" || !got.Cancelled {
					t.Fatal("SQLite close did not preserve cancellation", got)
				}
			}
			if len(f.requests) != 2 {
				t.Fatal("unexpected stop count", f.requests)
			}
		})
	}
}

func TestContentStopRetryRequiresDurableDefinitiveRejection(t *testing.T) {
	for _, owner := range []string{"roadmap", "test-generation"} {
		t.Run(owner, func(t *testing.T) {
			s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
			require(t, err)
			defer s.Close()
			ctx := context.Background()
			id, stopID := strings.Repeat("a", 32), strings.Repeat("b", 32)
			original := &runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: "original-stop", Input: json.RawMessage(`{"expected_revision":4}`)}
			replacement := &runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: "reobserved-stop", Input: json.RawMessage(`{"expected_revision":8}`)}
			revision := uint64(0)
			var save func(string, *runnercontract.Request) error
			if owner == "roadmap" {
				r := roadmap.Record{ID: id, RequestID: id, Project: "p", State: "cancelled", Cancelled: true, StopID: stopID, Request: runnercontract.Request{RequestID: "original-submit", Input: json.RawMessage(`{}`)}}
				save = func(state string, request *runnercontract.Request) error {
					r.Revision = revision + 1
					r.StopState = state
					r.StopRequest = request
					err := s.SaveRoadmap(ctx, r, revision)
					if err == nil {
						revision++
					}
					return err
				}
			} else {
				r := testmanagement.Generation{ID: id, Project: "p", State: "cancelled", Cancelled: true, StopID: stopID, Request: runnercontract.Request{RequestID: "original-submit", Input: json.RawMessage(`{}`)}}
				save = func(state string, request *runnercontract.Request) error {
					r.Revision = revision + 1
					r.StopState = state
					r.StopRequest = request
					err := s.SaveGeneration(ctx, r, revision)
					if err == nil {
						revision++
					}
					return err
				}
			}
			require(t, save("unknown", original))
			if save("unknown", replacement) == nil || save("unknown", nil) == nil {
				t.Fatal("uncertain stop envelope changed")
			}
			require(t, save("rejected", original))
			if save("unknown", nil) == nil {
				t.Fatal("rejected envelope was erased without a replacement")
			}
			require(t, save("unknown", replacement))
			require(t, save("accepted", replacement))
			if save("unknown", original) == nil {
				t.Fatal("accepted stop was replaced")
			}
		})
	}
}
