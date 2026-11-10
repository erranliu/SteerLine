package mainagentbridge

import (
	"context"
	"errors"
	"testing"

	"github.com/erranliu/SteerLine/internal/mainagent/contract"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestAdvisoryOriginalStopRequiresBoundReceiptOrPermanentStaleProof(t *testing.T) {
	for _, kind := range []string{"accepted", "missing-current", "missing-stale", "missing-future", "query-unknown", "foreign-request", "foreign-run", "foreign-command", "foreign-input", "empty-command"} {
		t.Run(kind, func(t *testing.T) {
			s, source, f := prepared(t)
			f.keepRunning = true
			step(t, s)
			r, _ := s.Get(context.Background(), SourceID(s.Project, source))
			r.StopRequest = runnerbinding.ContentStop("original-stop", 1)
			r.StopState = "unknown"
			if kind == "missing-stale" {
				f.run.Revision = 2
			}
			if kind == "missing-future" {
				r.StopRequest = runnerbinding.ContentStop("original-stop", 2)
			}
			state, err := InspectOriginalStop(context.Background(), r, &f.run, func(_ context.Context, selector, id string) (runnercontract.Inspection, error) {
				if selector != "request-id" || id != "original-stop" {
					t.Fatal("queried a replacement", selector, id)
				}
				if kind == "query-unknown" {
					return runnercontract.Inspection{}, &runnerclient.Failure{Code: "query_failed", Unknown: true}
				}
				if len(kind) >= 7 && kind[:7] == "missing" {
					return runnercontract.Inspection{}, &runnerclient.Failure{Code: "request_not_found", Exit: 5}
				}
				run := f.run
				command := runnercontract.Command{CommandID: "stop-command", RequestID: id, Command: "stop", RunID: &run.RunID, ReceiptStatus: "accepted", ActionStatus: "pending"}
				switch kind {
				case "foreign-request":
					command.RequestID = "other"
				case "foreign-run":
					run.RunID = "other"
				case "foreign-command":
					command.Command = "reply"
				case "foreign-input":
					run.InputDigest = "other"
				case "empty-command":
					command.CommandID = ""
				}
				return runnercontract.Inspection{Run: &run, Command: &command}, nil
			})
			want := map[string]string{"accepted": "accepted", "missing-current": "unknown", "missing-stale": "rejected"}[kind]
			if want == "" && err == nil || want != "" && (err != nil || state != want) {
				t.Fatal(kind, state, err)
			}
		})
	}
}

func TestAdvisoryUnknownStopDoesNotRetryWithoutStaleProof(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing-current", true: "accepted-pending"}[accepted], func(t *testing.T) {
			s, source, f := prepared(t)
			f.keepRunning = true
			step(t, s)
			f.run.ContentResult, f.run.Final = nil, nil
			r, _ := s.Get(context.Background(), SourceID(s.Project, source))
			if _, err := s.Cancel(context.Background(), r.ID, r.Revision); err != nil {
				t.Fatal(err)
			}
			f.stopFailure = func(request *runnercontract.Request) error {
				if accepted {
					f.stopAccepted = map[string]bool{request.RequestID: true}
				}
				return &runnerclient.Failure{Code: "lost_stop_reply", Unknown: true}
			}
			step(t, s)
			first, _ := s.Get(context.Background(), r.ID)
			step(t, s)
			step(t, s)
			last, _ := s.Get(context.Background(), r.ID)
			if f.stops != 1 || last.Settled || last.StopRequest.RequestID != first.StopRequest.RequestID || len(f.stopQueries) == 0 {
				t.Fatal(last, f.stops, f.stopQueries)
			}
			if accepted && last.StopState != "accepted" {
				t.Fatal(last)
			}
		})
	}
}

func TestAdvisoryCompletedWorkRejectsOldCancellation(t *testing.T) {
	s, source, f := prepared(t)
	step(t, s)
	shown, _ := s.Get(context.Background(), SourceID(s.Project, source))
	step(t, s)
	if _, err := s.Cancel(context.Background(), shown.ID, shown.Revision); err == nil || errors.Is(err, contract.ErrConflict) {
		t.Fatal("terminal guard was replaced by revision guard", err)
	}
	current, _ := s.Get(context.Background(), shown.ID)
	if current.State != "ready" || current.Cancelled || f.stops != 0 {
		t.Fatal(current)
	}
}
