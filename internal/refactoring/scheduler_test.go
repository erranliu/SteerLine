package refactoring

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	m "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

type delayedMaintenanceSubmit struct {
	Runner
	action  string
	blocked chan struct{}
}

func (r delayedMaintenanceSubmit) Submit(ctx context.Context, request runnercontract.Request) (runnerclient.Receipt, error) {
	var input runnercontract.GenerateRunInput
	_ = json.Unmarshal(request.Input, &input)
	if input.Correlation["steerline_refactoring"] == r.action {
		close(r.blocked)
		<-ctx.Done()
		return runnerclient.Receipt{}, ctx.Err()
	}
	return r.Runner.Submit(ctx, request)
}

func TestMaintenanceSlowSubmitDoesNotBlockSibling(t *testing.T) {
	s, runner, first := fixture(t)
	runner.hold = true
	s.Queue = runnerbinding.NewContentQueue(3)
	blocked := make(chan struct{})
	s.Runner = delayedMaintenanceSubmit{Runner: runner, action: first.ID, blocked: blocked}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	other, err := s.Command(ctx, m.Command{ID: m.ID(), Action: "refactoring_start", Target: s.Project, Expected: 1, Raw: json.RawMessage(`{}`)}, &first.Input)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx)
	}()
	defer func() { cancel(); <-done }()
	select {
	case <-blocked:
	case <-ctx.Done():
		t.Fatal("first submit never reached the barrier")
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		r, err := s.Store.Refactoring(ctx, s.Project, other.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Operations) > 0 && r.Operations[0].RunID != "" {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("sibling did not submit while first was blocked")
		case <-ticker.C:
		}
	}
}

func TestMaintenanceConcurrencySlotsAndClock(t *testing.T) {
	s, runner, first := fixture(t)
	ctx := context.Background()
	now := first.CreatedAt
	s.Now = func() time.Time { return now }
	s.Queue = runnerbinding.NewContentQueue(1)
	runner.hold = true
	records := []m.Record{first}
	for i := 0; i < 3; i++ {
		now = now.Add(time.Second)
		record, err := s.Command(ctx, m.Command{ID: m.ID(), Action: "refactoring_start", Target: s.Project, Expected: 1, Raw: json.RawMessage(`{}`)}, &first.Input)
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	get := func(index int) m.Record {
		t.Helper()
		r, err := s.Store.Refactoring(ctx, s.Project, records[index].ID)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	tick := func() {
		t.Helper()
		if err := s.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	now = now.Add(3 * time.Minute)
	work, err := s.scheduled(ctx, false)
	if err != nil || len(work) != 3 {
		t.Fatal("did not reserve three project slots", err)
	}
	for i := 0; i < 3; i++ {
		if r := get(i); !r.Slot || r.Elapsed(now) != 0 {
			t.Fatal("oldest actions must take slots without charging initial queue", r.ID)
		}
	}
	if r := get(3); r.Slot || r.Elapsed(now) != 0 || r.DisplayPhase() != "queued" {
		t.Fatal("fourth must remain queued", r.ID)
	}
	now = now.Add(2 * time.Minute)
	for _, record := range work {
		if err := s.advanceAction(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	tick()
	running, waiting := -1, -1
	for i := 0; i < 3; i++ {
		if get(i).Authorized {
			running = i
		} else {
			waiting = i
		}
	}
	if running < 0 || waiting < 0 {
		t.Fatal("expected one submitted action and capacity waiters")
	}
	if r := get(waiting); !r.Slot || r.ActiveSince != nil || r.WaitReason == "" || r.Elapsed(now) != 2*time.Minute {
		t.Fatal("shared capacity must pause budget while retaining project slot", r.ID, r.WaitReason, r.Elapsed(now))
	}
	now = now.Add(5 * time.Minute)
	tick()
	if r := get(waiting); r.Elapsed(now) != 2*time.Minute {
		t.Fatal("repeated waits charged", r.Elapsed(now))
	}
	// A waiting, unsent action releases both its own slot and shared queue entry.
	stopped := control(t, s, get(waiting), "refactoring_stop")
	if stopped.Slot || stopped.Phase != "stopped" || !stopped.Operations[0].Settled() {
		t.Fatal("unsent cancellation retained capacity", stopped.ID)
	}
	tick()
	if r := get(3); !r.Slot || r.Elapsed(now) != 0 {
		t.Fatal("queued action did not take the next available slot", r.ID)
	}
	// A submitted action still holds its own slot until the original stops.
	stopping := control(t, s, get(running), "refactoring_stop")
	if !stopping.Slot || stopping.Phase != "stopping" {
		t.Fatal("submitted action released prematurely", stopping.ID)
	}
}

func TestMaintenanceTimeBudgetExcludesQueueAndStopSettlement(t *testing.T) {
	s, runner, first := fixture(t)
	ctx := context.Background()
	now := first.CreatedAt
	s.Now = func() time.Time { return now }
	runner.hold = true
	s.Queue.UpdateContent("other-project", "busy", now, true, false)
	if _, err := s.update(ctx, first.ID, func(r *m.Record) error {
		r.Input.Budget = m.Budget{Minutes: "6"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(3 * time.Minute)
	work, err := s.scheduled(ctx, false)
	if err != nil || len(work) != 1 {
		t.Fatal("missing project slot", err)
	}
	now = now.Add(2 * time.Minute)
	if err := s.advanceAction(ctx, work[0]); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	now = now.Add(5 * time.Minute)
	for i := 0; i < 2; i++ {
		if err := s.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	s.Queue.UpdateContent("other-project", "busy", now, true, true)
	running := tickUntil(t, s, first.ID, func(r m.Record) bool { return r.Authorized })
	if running.Elapsed(now) != 2*time.Minute {
		t.Fatal("preparation clock was lost or queue was charged", running.Elapsed(now))
	}
	now = now.Add(4 * time.Minute)
	running, err = s.Store.Refactoring(ctx, s.Project, first.ID)
	if err != nil || !running.BudgetExhausted(now) {
		t.Fatal("exact six minute budget should be exhausted", err)
	}
	control(t, s, running, "refactoring_stop")
	now = now.Add(2 * time.Minute)
	stopped := tickUntil(t, s, first.ID, func(r m.Record) bool { return r.Phase == "stopped" })
	if stopped.Slot || stopped.FinishedAt == nil || stopped.FinishedAt.Sub(stopped.CreatedAt) != 16*time.Minute || stopped.Elapsed(now) != 6*time.Minute {
		t.Fatal("elapsed and settlement intervals are incorrect", stopped.Elapsed(now), stopped.FinishedAt)
	}
}

func TestMaintenanceFirstDispatchChecksQualificationAfterQueue(t *testing.T) {
	s, runner, r := fixture(t)
	s.Qualify = func(context.Context, m.Record) (env.Admission, error) {
		return env.Admission{}, m.ErrEnvironmentChanged
	}
	blocked := tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "blocked" })
	if blocked.Authorized || blocked.Slot || blocked.CanFinish() || len(runner.requests) != 0 {
		t.Fatal("revoked environment received work", blocked.ID)
	}
}

func TestMaintenanceQueuedConflictStopsWhenOriginalPRCloses(t *testing.T) {
	s, runner, first := fixture(t)
	delivered := tickUntil(t, s, first.ID, func(r m.Record) bool { return r.Phase == "delivered" })
	started := len(runner.requests)
	s.Verify = func(_ context.Context, r m.Record, _ m.Result) (*runnercontract.PRFact, string, error) {
		fact := *r.PR
		if r.ConflictPending {
			fact.State = "closed"
			return &fact, "", nil
		}
		return &fact, "", nil
	}
	control(t, s, delivered, "refactoring_conflict")
	result := tickUntil(t, s, first.ID, func(r m.Record) bool { return r.Phase == "delivered" })
	if len(runner.requests) != started || len(result.Reports) != len(delivered.Reports) || result.PR.URL != delivered.PR.URL || result.PR.State != "closed" || result.Slot {
		t.Fatal("queued conflict lost original delivery or dispatched work", result.ID)
	}
}

func TestMaintenanceTransientQualificationYieldsSlot(t *testing.T) {
	s, _, first := fixture(t)
	ctx := context.Background()
	now := first.CreatedAt
	s.Now = func() time.Time { return now }
	s.Qualify = func(context.Context, m.Record) (env.Admission, error) {
		return env.Admission{}, errors.New("current qualification unavailable")
	}
	if err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	wait, err := s.Store.Refactoring(ctx, s.Project, first.ID)
	if err != nil || wait.Slot || wait.DisplayPhase() != "queued" || wait.ActiveSince != nil {
		t.Fatal("unknown qualification kept project capacity", wait.ID, err)
	}
	now = now.Add(time.Minute)
	if err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	blocked, err := s.Store.Refactoring(ctx, s.Project, first.ID)
	if err != nil || blocked.Phase != "blocked" || blocked.Slot || blocked.Elapsed(now) != 0 {
		t.Fatal("bounded verification did not block or charged wait", blocked.ID, err)
	}
}
