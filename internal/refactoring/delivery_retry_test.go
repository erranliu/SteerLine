package refactoring

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	m "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestMaintenanceRetriesRejectedPublicationFromAcceptedWork(t *testing.T) {
	s, f, r := fixture(t)
	f.rejectPublish = true
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "blocked" })
	if r.FinishAction() != "retry_publish" || r.PublicationUnverified() {
		t.Fatal("rejected publication must allow delivery retry", r.FinishAction())
	}
	before := len(f.requests)
	r = control(t, s, r, "refactoring_finish")
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
	if r.PR == nil || len(f.requests) != before+2 || r.FinishAction() != "" {
		t.Fatal("retry did not publish and review once", r.Phase, len(f.requests)-before)
	}
}

func TestMaintenanceRetriesPRVerificationAndReviewWithoutPublishing(t *testing.T) {
	for _, phase := range []string{"verifying", "report", "report-completed"} {
		t.Run(phase, func(t *testing.T) {
			s, f, r := fixture(t)
			r = tickUntil(t, s, r.ID, func(r m.Record) bool {
				if phase == "report-completed" && len(r.Operations) > 0 {
					last := r.Operations[len(r.Operations)-1]
					return r.Phase == "report" && last.Purpose == "report" && last.RunID != ""
				}
				return r.Phase == phase
			})
			verify := s.Verify
			s.Verify = func(context.Context, m.Record, m.Result) (*runnercontract.PRFact, string, error) {
				return nil, "", errors.New("fixture PR query unavailable")
			}
			r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "blocked" })
			want := "retry_review"
			if phase == "verifying" {
				want = "verify_pr"
			}
			if r.FinishAction() != want || r.Unsettled() || r.Diagnostic != "fixture PR query unavailable" {
				t.Fatal("failed PR query has no usable recovery", r.FinishAction(), r.Diagnostic)
			}
			before := len(f.requests)
			s.Verify = verify
			r = control(t, s, r, "refactoring_finish")
			r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
			if r.PR == nil || len(f.requests) != before+1 {
				t.Fatal("retry must only review the existing publication", r.Phase, len(f.requests)-before)
			}
			var input runnercontract.GenerateRunInput
			if err := json.Unmarshal(f.requests[before].Input, &input); err != nil || input.Workflow.ID != "generate" {
				t.Fatal("recovery launched a writer", err)
			}
		})
	}
}

func TestMaintenanceRechecksOriginalPublicationRun(t *testing.T) {
	s, f, r := fixture(t)
	f.malformedPublication = true
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "blocked" })
	if r.FinishAction() != "verify_publication" {
		t.Fatal(r.FinishAction())
	}
	before := len(f.requests)
	original := r.Operations[len(r.Operations)-1]
	result := m.Result{ActionID: r.ID, OperationID: original.ID, Outcome: "done",
		PR: "https://example.test/repo/pull/1", Head: strings.Repeat("a", 40),
		Base: strings.Repeat("b", 40), Summary: "原发布结果已核实"}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	f.runs[original.RunID].Revision++
	f.runs[original.RunID].Final.Text = string(raw)
	r = control(t, s, r, "refactoring_finish")
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
	if r.PR == nil || len(f.requests) != before+1 || len(r.Reports) != 1 ||
		r.Operations[len(r.Operations)-2].ID != original.ID {
		t.Fatal("recheck replaced the original publication", r.Phase, len(f.requests)-before)
	}
}

func TestMaintenancePRQueryFailureAfterMissingDiffOffersReviewRetry(t *testing.T) {
	s, _, r := fixture(t)
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "report" })
	s.ReviewDiff = func(context.Context, m.Record, runnercontract.PRFact) (string, error) {
		return "", errors.New("fixture missing diff")
	}
	verify, calls := s.Verify, 0
	s.Verify = func(ctx context.Context, record m.Record, result m.Result) (*runnercontract.PRFact, string, error) {
		calls++
		if calls == 2 {
			return nil, "", errors.New("fixture PR query unavailable")
		}
		return verify(ctx, record, result)
	}
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "blocked" })
	if calls != 2 || r.FinishAction() != "retry_review" {
		t.Fatal("query failure after missing diff stayed active", calls, r.Phase, r.FinishAction())
	}
	s.Verify = verify
	r = control(t, s, r, "refactoring_finish")
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
	if r.PR == nil || len(r.Reports) != 1 || r.Reports[0].Recommendation != "insufficient_evidence" {
		t.Fatal("review retry lost the evidence limitation", r.Phase, r.Reports)
	}
}

func TestMaintenanceNoChangesMustHaveEqualHeadAndBase(t *testing.T) {
	s, _, r := fixture(t)
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "audit" })
	r, err := s.update(context.Background(), r.ID, func(r *m.Record) error {
		r.Phase = "verifying"
		r.Candidate = &m.Result{NoChanges: true, Outcome: "done", Head: strings.Repeat("a", 40), Base: strings.Repeat("b", 40)}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "blocked" })
	if r.PR != nil || r.FinishAction() != "verify_pr" {
		t.Fatal("different revisions were presented as completion without changes")
	}
}
