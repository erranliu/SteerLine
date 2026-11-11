package runnerbridge

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnerclient"
)

type snapshotFixture struct {
	*fixtureCLI
	result runnerclient.RunObservation
	calls  int
	after  string
}

type stopObservationFixture struct {
	*fixtureCLI
	calls      []string
	requests   []runnerclient.Request
	stopError  string
	observeErr error
}

func (f *stopObservationFixture) Inspect(ctx context.Context, selector, id string) (runnerclient.Inspection, error) {
	f.calls = append(f.calls, "inspect "+selector+" "+id)
	return f.fixtureCLI.Inspect(ctx, selector, id)
}

func (f *stopObservationFixture) Call(ctx context.Context, command string, args []string, req *runnerclient.Request) (runnerclient.Response, error) {
	f.calls = append(f.calls, command+" "+req.RequestID)
	f.requests = append(f.requests, *req)
	switch f.stopError {
	case "stale_revision":
		f.run.Revision++
		return runnerclient.Response{}, &runnerclient.Failure{Code: "stale_revision", Message: "Runner revision changed", Exit: 5}
	case "request_outcome_unknown":
		_, _ = f.fixtureCLI.Call(ctx, command, args, req)
		return runnerclient.Response{}, &runnerclient.Failure{Code: "request_outcome_unknown", Exit: 6, Unknown: true}
	}
	return f.fixtureCLI.Call(ctx, command, args, req)
}

func TestResumeRejectionPersistsRunnerDiagnostic(t *testing.T) {
	b, s, plain := setup()
	s.value.RunID, s.value.SubmitState = "run", "accepted"
	s.value.Actions = []Action{{
		Command: "resume", State: "unknown",
		Request: runnerclient.Request{RequestID: "original-resume"},
	}}
	f := &stopObservationFixture{fixtureCLI: plain, stopError: "stale_revision"}
	b.Client = func(string) CLI { return f }
	err := b.Step(context.Background(), "project", "candidate")
	action := s.value.Actions[0]
	if err == nil || action.State != "rejected" || action.ErrorCode != "stale_revision" ||
		action.RejectionDiagnostic != "Runner revision changed" || action.Request.RequestID != "original-resume" {
		t.Fatalf("rejection lost its original diagnostic: %+v, %v", action, err)
	}
}

func (f *stopObservationFixture) Observe(_ context.Context, id, after string) (runnerclient.RunObservation, error) {
	f.calls = append(f.calls, "observe "+id+" "+after)
	return runnerclient.RunObservation{Contract: runnerclient.RunObservationContract, Run: f.run, EventPage: runnerclient.EventPage{NextCursor: "after-stop", Events: []runnerclient.Event{{EventID: "stop-event", RunID: id, Revision: f.run.Revision}}}}, f.observeErr
}

func TestStopObservationReadsNewRevisionAndKeepsOriginalRequest(t *testing.T) {
	for _, mode := range []string{"stale_revision", "request_outcome_unknown", "observe_failed"} {
		t.Run(mode, func(t *testing.T) {
			b, s, plain := setup()
			s.value.RunID, s.value.SubmitState = "run", "accepted"
			s.value.StopRequested, s.value.Cursor = true, "before-stop"
			f := &stopObservationFixture{fixtureCLI: plain, stopError: mode}
			if mode == "observe_failed" {
				f.observeErr = errors.New("observation unavailable")
			}
			b.Client = func(string) CLI { return f }
			step := func() error { return b.Step(context.Background(), "project", "candidate") }
			if err := step(); err == nil || len(f.requests) != 1 || s.value.Cursor != "before-stop" || len(s.events) != 0 || s.value.Stop == nil {
				t.Fatal("failed step advanced observation or lost the original stop", s.value, err)
			}
			original := f.requests[0].RequestID
			want := []string{"inspect run-id run", "inspect request-id " + original, "stop " + original}
			if mode == "observe_failed" {
				want = append(want, "observe run before-stop")
				if s.value.Stop.State != "accepted" || s.value.Projection.Revision != 1 {
					t.Fatal("read failure lost the accepted receipt or published an unseen revision", s.value)
				}
			}
			if !slices.Equal(f.calls, want) || string(f.requests[0].Input) != `{"expected_revision":1}` {
				t.Fatal("unexpected original stop sequence", f.calls, f.requests)
			}
			f.stopError, f.observeErr, f.calls = "", nil, nil
			if err := step(); err != nil {
				t.Fatal(err)
			}
			want = []string{"inspect run-id run"}
			switch mode {
			case "stale_revision":
				if len(f.requests) != 2 || f.requests[1].RequestID == original || string(f.requests[1].Input) != `{"expected_revision":2}` {
					t.Fatal("known rejection did not prepare a new request from the freshly read revision", f.requests)
				}
				want = append(want, "inspect request-id "+f.requests[1].RequestID, "stop "+f.requests[1].RequestID)
			case "request_outcome_unknown":
				want = append(want, "inspect request-id "+original)
			}
			want = append(want, "observe run before-stop")
			if !slices.Equal(f.calls, want) || f.stops != 1 || s.value.Stop.State != "accepted" || s.value.Stop.Request.RequestID != f.requests[len(f.requests)-1].RequestID || s.value.Projection.Revision != f.run.Revision || s.value.Cursor != "after-stop" || !s.events["stop-event"] {
				body, _ := json.Marshal(s.value)
				t.Fatal("stop receipt or its post-control observation was not preserved", f.calls, string(body))
			}
		})
	}
}

func (f *snapshotFixture) Observe(_ context.Context, _, after string) (runnerclient.RunObservation, error) {
	f.calls++
	f.after = after
	return f.result, nil
}

func TestRunnerObservationAtomicSave(t *testing.T) {
	for _, mode := range []string{"valid", "future-event", "wrong-input", "wrong-run", "missing-contract", "cas-failure"} {
		t.Run(mode, func(t *testing.T) {
			b, s, plain := setup()
			s.value.RunID, s.value.SubmitState = "run", "accepted"
			s.value.InputDigest = "input"
			f := &snapshotFixture{fixtureCLI: plain, result: runnerclient.RunObservation{Contract: runnerclient.RunObservationContract, Run: plain.run, EventPage: runnerclient.EventPage{NextCursor: "next", Events: []runnerclient.Event{{EventID: "one", RunID: "run", Revision: 1}}}}}
			switch mode {
			case "future-event":
				f.result.EventPage.Events[0].Revision = 2
			case "wrong-input":
				f.result.Run.InputDigest = "foreign"
			case "wrong-run":
				f.result.Run.RunID = "foreign"
			case "missing-contract":
				f.result.Contract = ""
			case "cas-failure":
				s.fail = true
			}
			b.Client = func(string) CLI { return f }
			err := b.Step(context.Background(), "project", "candidate")
			if mode == "valid" {
				if err != nil || s.value.Cursor != "next" || !s.events["one"] || s.value.Projection == nil {
					t.Fatal(s.value, err)
				}
			} else if err == nil || s.value.Cursor != "" || len(s.events) != 0 || s.value.Projection != nil {
				t.Fatal("invalid/CAS page partially saved", mode, s.value, err)
			}
			if f.calls != 1 || f.reads != 0 || f.inputs != 0 || f.submits != 0 {
				t.Fatal("observation made extra calls or side effects", f)
			}
		})
	}
}

func TestRunnerObservationContinuesFromLastCommittedCursor(t *testing.T) {
	b, s, plain := setup()
	s.value.RunID, s.value.SubmitState = "run", "accepted"
	f := &snapshotFixture{fixtureCLI: plain, result: runnerclient.RunObservation{Contract: runnerclient.RunObservationContract, Run: plain.run}}
	b.Client = func(string) CLI { return f }
	step := func(after string, page runnerclient.EventPage, fail bool) {
		t.Helper()
		f.result.EventPage, s.fail = page, fail
		if err := b.Step(context.Background(), "project", "candidate"); (err != nil) != fail || f.after != after {
			t.Fatalf("observe from %q, want %q: %v", f.after, after, err)
		}
	}
	event := func(id string, revision uint64) []runnerclient.Event {
		return []runnerclient.Event{{EventID: id, RunID: "run", Revision: revision}}
	}
	step("", runnerclient.EventPage{NextCursor: "page-1", HasMore: true, Events: event("first", 1)}, false)
	second := runnerclient.EventPage{NextCursor: "page-2", Events: event("second", 1)}
	step("page-1", second, true)
	if s.value.Cursor != "page-1" || s.events["second"] {
		t.Fatal("failed commit consumed a page")
	}
	step("page-1", second, false)
	step("page-2", runnerclient.EventPage{NextCursor: "page-2", Events: []runnerclient.Event{}}, false)
	f.result.Run.Revision++
	step("page-2", runnerclient.EventPage{NextCursor: "page-3", Events: event("third", 2)}, false)
	f.result.Run.Revision--
	if err := b.Step(context.Background(), "project", "candidate"); err == nil || s.value.Projection.Revision != 2 || s.value.Cursor != "page-3" || len(s.events) != 3 || f.reads != 0 {
		t.Fatal("rewind or pagination changed committed observation", s.value, s.events, err)
	}
}
