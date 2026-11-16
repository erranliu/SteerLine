package refactoring

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/refactoring/model"
)

func TestMaintenanceAcceptsBeforeStartupChecks(t *testing.T) {
	s, runner, ready := fixture(t)
	ctx := t.Context()
	control(t, s, ready, "refactoring_stop")
	prepare := s.Prepare
	entered := make(chan struct{})
	release := make(chan struct{})
	s.Prepare = func(ctx context.Context, in model.Start) (model.Frozen, error) {
		close(entered)
		select {
		case <-ctx.Done():
			return model.Frozen{}, ctx.Err()
		case <-release:
			return prepare(ctx, in)
		}
	}
	c := model.Command{ID: model.ID(), Action: "refactoring_start", Target: s.Project, Expected: 1, Raw: json.RawMessage(`{"start":true}`)}
	r, err := s.Command(ctx, c, &ready.Input)
	if err != nil || r.ID != c.ID || r.DisplayPhase() != model.PhaseCheckingStart || r.Slot {
		t.Fatalf("unexpected startup receipt: phase=%q slot=%t error=%v", r.DisplayPhase(), r.Slot, err)
	}
	replayed, err := s.Command(ctx, c, &ready.Input)
	if err != nil || replayed.ID != r.ID || replayed.Revision != r.Revision {
		t.Fatalf("replay did not return the original receipt: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Tick(ctx) }()
	defer close(release)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not begin startup checks")
	}
	stored, err := s.Store.Refactoring(ctx, s.Project, r.ID)
	if err != nil || stored.DisplayPhase() != model.PhaseCheckingStart || stored.Slot || stored.ActiveSince != nil || stored.Elapsed(time.Now()) != 0 {
		t.Fatalf("checking must have no execution slot or budget clock: %v", err)
	}
	control(t, s, stored, "refactoring_stop")
	// Let the stale check succeed after the user has stopped the action.
	release <- struct{}{}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	stored, err = s.Store.Refactoring(ctx, s.Project, r.ID)
	if err != nil || stored.Phase != "stopped" || len(runner.requests) != 0 {
		t.Fatalf("a stopped action was revived or dispatched: phase=%q error=%v", stored.Phase, err)
	}
}

func TestMaintenanceStartupFailureRemainsOnOriginalAction(t *testing.T) {
	s, runner, ready := fixture(t)
	ctx := t.Context()
	control(t, s, ready, "refactoring_stop")
	s.Prepare = func(context.Context, model.Start) (model.Frozen, error) {
		return model.Frozen{}, errors.New("环境资格已变化")
	}
	c := model.Command{ID: model.ID(), Action: "refactoring_start", Target: s.Project, Expected: 1, Raw: json.RawMessage(`{}`)}
	r, err := s.Command(ctx, c, &ready.Input)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	stored := tickUntil(t, s, r.ID, func(r model.Record) bool { return r.Phase == "blocked" || !r.Active() })
	if stored.Phase != "blocked" || stored.Diagnostic != "环境资格已变化" || stored.Input != ready.Input ||
		stored.Slot || stored.Elapsed(time.Now()) != 0 || len(runner.requests) != 0 {
		t.Fatalf("startup failure did not settle the original action: phase=%q diagnostic=%q", stored.Phase, stored.Diagnostic)
	}
}
