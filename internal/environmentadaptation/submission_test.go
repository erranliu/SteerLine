package environmentadaptation_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation"
	envmodel "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
)

type cancellingSubmit struct {
	*runner
	cancel context.CancelFunc
}

func (r cancellingSubmit) Call(ctx context.Context, command string, args []string, request *runnerclient.Request) (runnerclient.Response, error) {
	response, err := r.runner.Call(ctx, command, args, request)
	if command == "submit" {
		r.cancel()
	}
	return response, err
}

func TestEnvironmentSubmissionPersistsOutcomeAfterCancellation(t *testing.T) {
	for _, outcome := range []string{"accepted", "unknown", "rejected"} {
		t.Run(outcome, func(t *testing.T) {
			s, remote, _, _ := setup(t)
			r := start(t, s)
			if outcome != "accepted" {
				remote.submitFailure = &runnerclient.Failure{Code: "original_submit_failure", Message: "first cause", Exit: 5, Unknown: outcome == "unknown"}
			}
			caller, cancel := context.WithCancel(ctx)
			defer cancel()
			s.Runner = cancellingSubmit{remote, cancel}
			err := s.Observe(caller)
			if outcome == "unknown" && err == nil {
				t.Fatal("missing unknown submit error")
			}
			saved := read(t, s, r.ID)
			a := saved.Rounds[0]
			if a.Submission != outcome || remote.submits != 1 {
				t.Fatal("lost original submission outcome", a.Submission, remote.submits, err)
			}
			if outcome != "accepted" && a.SubmissionDiagnostic != remote.submitFailure.Error() {
				t.Fatal("lost first submit failure", a.SubmissionDiagnostic, err)
			}
			if outcome == "rejected" && !saved.Settled() {
				t.Fatal("known rejection did not settle")
			}
		})
	}
}

func TestEnvironmentSubmissionDiagnosticSurvivesRestartAndReconciliation(t *testing.T) {
	s, remote, store, _ := setup(t)
	r := start(t, s)
	remote.submitFailure = &runnerclient.Failure{Code: "request_outcome_unknown", Message: "original submit timed out", Unknown: true}
	if err := s.Observe(ctx); err == nil {
		t.Fatal("missing submit error")
	}
	original := remote.submitFailure.Error()
	s = &env.Service{Project: s.Project, Store: store, Runner: remote, Now: s.Now, Prepare: s.Prepare}
	remote.inspectFailure = &runnerclient.Failure{Code: "request_not_found", Message: "The original command was not found.", Exit: 5}
	for range 2 {
		if err := s.Observe(ctx); err == nil {
			t.Fatal("missing lookup failure")
		}
	}
	saved := read(t, s, r.ID)
	if saved.Rounds[0].SubmissionDiagnostic != original || saved.Diagnostic != remote.inspectFailure.Error() || !strings.Contains(saved.DiagnosticText(), original) || !strings.Contains(saved.DiagnosticText(), "request_not_found") || remote.submits != 1 {
		t.Fatal("submit cause overwritten or original resubmitted", saved.DiagnosticText(), remote.submits)
	}
	// Storage also refuses to clear or replace the original cause.
	for _, replacement := range []string{"", "replacement"} {
		altered := saved
		altered.Rounds = append([]envmodel.Round(nil), saved.Rounds...)
		altered.Rounds[0].SubmissionDiagnostic = replacement
		altered.Revision++
		if err := store.SaveEnvironment(ctx, altered, saved.Revision); err == nil {
			t.Fatal("original submit failure was mutable")
		}
	}
	remote.inspectFailure = nil
	setReport(remote, report(r, remote, true))
	observe(t, s)
	observe(t, s)
	saved = read(t, s, r.ID)
	if saved.Status() != "ready" || saved.Rounds[0].SubmissionDiagnostic != original || saved.DiagnosticText() != saved.Rounds[0].Report.Message || remote.submits != 1 {
		t.Fatal("recovery lost the cause or retained a stale public failure", saved.DiagnosticText())
	}
	// The readiness projection carries both causes while still unverified.
	saved.State, saved.Rounds[0].Settled = "running", false
	saved.Diagnostic = "request_not_found"
	_, _, reason := envmodel.View(envmodel.Current{}, saved, "", nil)
	if !strings.Contains(reason, original) || !strings.Contains(reason, "request_not_found") {
		t.Fatal("readiness projection hid the first cause", reason)
	}
}

type cancelBoundaryStore struct {
	env.Store
	cancel      context.CancelFunc
	beforeWrite bool
}

func (s cancelBoundaryStore) SaveEnvironment(ctx context.Context, r envmodel.Record, expected uint64) error {
	if s.beforeWrite && r.Rounds[0].Submission == "unknown" {
		s.cancel()
		return ctx.Err()
	}
	err := s.Store.SaveEnvironment(ctx, r, expected)
	if err == nil && r.Rounds[0].Submission == "unknown" {
		s.cancel()
	}
	return err
}

func TestEnvironmentCancellationBeforeSubmitDoesNotCreateUnknownOperation(t *testing.T) {
	s, remote, _, _ := setup(t)
	r := start(t, s)
	caller, cancel := context.WithCancel(ctx)
	defer cancel()
	s.Store = cancelBoundaryStore{Store: s.Store, cancel: cancel}
	_ = s.Observe(caller)
	saved := read(t, s, r.ID)
	if remote.submits != 0 || !saved.Settled() || saved.Rounds[0].Submission != "rejected" || saved.Rounds[0].SubmissionDiagnostic != context.Canceled.Error() {
		t.Fatal("unattempted submission became unknown", saved, remote.submits)
	}
	if !errors.Is(caller.Err(), context.Canceled) {
		t.Fatal("test did not cancel at submission boundary")
	}
}

func TestEnvironmentFailedSubmissionBoundaryRemainsPending(t *testing.T) {
	s, remote, store, _ := setup(t)
	r := start(t, s)
	caller, cancel := context.WithCancel(ctx)
	defer cancel()
	s.Store = cancelBoundaryStore{Store: store, cancel: cancel, beforeWrite: true}
	if err := s.Observe(caller); err == nil {
		t.Fatal("missing failed boundary write")
	}
	saved := read(t, s, r.ID)
	if remote.submits != 0 || saved.Rounds[0].Submission != "pending" || saved.Rounds[0].SubmissionDiagnostic != "" {
		t.Fatal("failed boundary created an unknown external operation", saved, remote.submits)
	}
	s.Store = store
	observe(t, s)
	if remote.submits != 1 || read(t, s, r.ID).Rounds[0].Submission != "accepted" {
		t.Fatal("unsent intent did not dispatch on a fresh observation")
	}
}
