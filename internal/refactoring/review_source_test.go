package refactoring

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	m "github.com/erranliu/SteerLine/internal/refactoring/model"
	rc "github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestReviewEvidenceOnlyIncludesAcceptedMatchingHead(t *testing.T) {
	for _, change := range []string{"matching", "old-head", "not-accepted", "failed", "dirty"} {
		t.Run(change, func(t *testing.T) {
			r := auditRecord()
			r.Phase = "report"
			r.PR = &rc.PRFact{HeadRevision: strings.Repeat("a", 40)}
			r.Operations[1].AcceptedOperation = "write"
			switch change {
			case "old-head":
				r.PR.HeadRevision = strings.Repeat("c", 40)
			case "not-accepted":
				r.Operations[1].AcceptedOperation = ""
			case "failed":
				r.Operations[0].Run.Status = "failed"
			case "dirty":
				*r.Operations[0].WorkspaceGit.Clean = false
			}
			facts := (&Service{}).context(r, m.ReviewEvidence{Checks: "checks"})
			if got := len(facts.Feedback); (got == 1) != (change == "matching") {
				t.Fatalf("%s: %+v", change, facts)
			}
			if change == "matching" && (facts.Feedback[0].Feedback != r.Operations[0].Feedback || facts.Feedback[0].WorkspaceGit != r.Operations[0].WorkspaceGit || facts.Checks != "checks") {
				t.Fatal("review lost original claims or Git facts")
			}
		})
	}
}

func TestStopCancelsReviewSourceBeforeModelSubmission(t *testing.T) {
	s, runner, r := fixture(t)
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
	r = control(t, s, r, "refactoring_review")
	before := len(runner.requests)
	started := make(chan struct{})
	s.Investigate = func(ctx context.Context, _ m.Record, _ string) error {
		close(started)
		<-ctx.Done()
		return nil
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Tick(ctx) }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("review source preparation did not start")
	}
	control(t, s, r, "refactoring_stop")
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("review source preparation ignored stop")
	}
	tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "stopped" })
	if len(runner.requests) != before {
		t.Fatal("stopped review source dispatched a model request")
	}
}

func TestReviewSourcePreparationFailureNeverDispatchesReport(t *testing.T) {
	s, runner, r := fixture(t)
	s.Investigate = func(_ context.Context, current m.Record, _ string) error {
		if current.Phase == "report" {
			return errors.New("pinned review source unavailable")
		}
		return nil
	}
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "blocked" })
	if !strings.Contains(r.Diagnostic, "pinned review source unavailable") || r.Operations[len(r.Operations)-1].Purpose != "report" {
		t.Fatal("missing source was not explicit", r.Phase, r.Diagnostic)
	}
	for _, req := range runner.requests {
		var input rc.GenerateRunInput
		if err := json.Unmarshal(req.Input, &input); err != nil {
			t.Fatal(err)
		}
		if input.Title == "重构与维护 · report" {
			t.Fatal("review without pinned source reached Runner")
		}
	}
}
