package refactoring

import (
	"context"
	"testing"

	m "github.com/erranliu/SteerLine/internal/refactoring/model"
)

func TestMaintenanceObservesBlockedRunWithoutNewInput(t *testing.T) {
	s, f, r := fixture(t)
	f.hold = true
	r = tickUntil(t, s, r.ID, func(r m.Record) bool {
		return len(r.Operations) == 1 && r.Operations[0].RunID != ""
	})
	id := r.Operations[0].RunID
	run := f.runs[id]
	run.Revision++
	run.Status, run.Phase, run.Diagnostic = "blocked", "manual_intervention_required", "app-server connection refused"
	for range 3 {
		if err := s.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	r, err := s.Store.Refactoring(context.Background(), s.Project, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if r.DisplayPhase() != "blocked" || r.Phase != "first" || !r.Active() || !r.Slot || !r.Unsettled() || r.CanFinish() ||
		len(f.requests) != 1 || f.stops != 0 || r.Operations[0].CompletedAt != nil {
		t.Fatalf("blocked run was hidden, settled or resubmitted: %+v", r)
	}
	command := m.Command{ID: m.ID(), Action: "refactoring_finish", Target: r.ID, Expected: r.Revision, Raw: []byte(`{}`)}
	if _, err := s.Command(context.Background(), command, nil); err == nil {
		t.Fatal("blocked unsettled run accepted early delivery")
	}
	// A later Runner observation can clear the block without losing the original
	// workflow phase. This service does not send a resume or replacement submit.
	run.Revision++
	run.Status, run.Phase, run.Diagnostic = "running", "", ""
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	r, err = s.Store.Refactoring(context.Background(), s.Project, r.ID)
	if err != nil || r.DisplayPhase() != "first" || len(f.requests) != 1 || f.stops != 0 {
		t.Fatalf("later original observation failed to clear block: %+v, %v", r, err)
	}
	r = control(t, s, r, "refactoring_stop")
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "stopped" })
	if f.stops != 1 || len(f.requests) != 1 || r.Unsettled() {
		t.Fatal("original stop duties were lost")
	}
}
