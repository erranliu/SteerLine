package testmanagement

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerclient"
)

type contextGenerationStore struct {
	*memoryGenerations
	cancelBoundary context.CancelFunc
}

func (m contextGenerationStore) SaveGeneration(ctx context.Context, r Generation, expected uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	err := m.memoryGenerations.SaveGeneration(ctx, r, expected)
	if err == nil && r.State == "unknown" && m.cancelBoundary != nil {
		m.cancelBoundary()
	}
	return err
}

type cancelledGenerationSubmit struct {
	*generationCLI
	cancel context.CancelFunc
}

func (f cancelledGenerationSubmit) Submit(ctx context.Context, request runnerclient.Request) (runnerclient.Receipt, error) {
	receipt, err := f.generationCLI.Submit(ctx, request)
	f.cancel()
	return receipt, err
}

func TestTMGenerationSubmissionOutcomeSurvivesCallerCancellation(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "accepted", true: "unknown"}[unknown], func(t *testing.T) {
			s, store, runner, in := generationFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s.Store = contextGenerationStore{memoryGenerations: store}
			runner.lost = unknown
			s.Runner = cancelledGenerationSubmit{generationCLI: runner, cancel: cancel}
			returned, err := s.Generate(ctx, in)
			saved, readErr := store.Generation(context.Background(), s.Project, in.RequestID)
			if readErr != nil || !reflect.DeepEqual(returned, saved) || saved.Revision != 3 || runner.submits != 1 {
				t.Fatal("submission outcome was lost at the caller deadline", returned.State, saved.State, err, readErr)
			}
			if unknown {
				if err == nil || saved.State != "unknown" || !strings.Contains(saved.SubmissionDiagnostic, "request_outcome_unknown") {
					t.Fatal("original unknown cause was not preserved", saved.State, saved.SubmissionDiagnostic, err)
				}
			} else if err != nil || saved.State != "accepted" || saved.RunID == "" {
				t.Fatal("accepted receipt was not preserved", saved.State, saved.RunID, err)
			}
			s.Runner = runner
			if err := s.Observe(context.Background()); err != nil || runner.submits != 1 {
				t.Fatal("recovery duplicated the original submission", err, runner.submits)
			}
		})
	}
}

func TestTMGenerationCancelledBeforeExternalSubmitIsDefinitive(t *testing.T) {
	s, store, runner, in := generationFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Store = contextGenerationStore{memoryGenerations: store, cancelBoundary: cancel}
	_, err := s.Generate(ctx, in)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("missing original cancellation", err)
	}
	saved, err := store.Generation(context.Background(), s.Project, in.RequestID)
	if err != nil || runner.submits != 0 || saved.State != "rejected" || !saved.Settled() || saved.SubmissionDiagnostic == "" {
		t.Fatal("unattempted submission became an uncertain external operation", saved.State, saved.SubmissionDiagnostic, runner.submits, err)
	}
}

func TestTMGenerationExpiredDispatchLeavesQueuedIntent(t *testing.T) {
	s, store, runner, in := generationFixture(t)
	s.Queue = NewGenerationQueue(1)
	s.Queue.Track("other", "busy", time.Now().UTC(), true)
	g, err := s.Generate(context.Background(), in)
	if err != nil || !g.Unsubmitted() {
		t.Fatal(g.State, err)
	}
	s.Queue.Release("other", "busy")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = s.dispatch(ctx, g)
	saved, _ := store.Generation(context.Background(), s.Project, g.ID)
	if !saved.Unsubmitted() || runner.submits != 0 || saved.Revision != g.Revision {
		t.Fatal("expired slice crossed the external ambiguity boundary", saved.State, runner.submits)
	}
}
