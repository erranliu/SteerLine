package testmanagement

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestGenerationQueueSharedLimitAndFIFO(t *testing.T) {
	ctx := context.Background()
	queue := NewGenerationQueue(5)
	var services []*GenerationService
	var runners []*generationCLI
	for n := 0; n < 7; n++ {
		s, _, runner, in := generationFixture(t)
		s.Project, s.Queue = fmt.Sprint("project-", n), queue
		g, err := s.Generate(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		want := "accepted"
		if n >= 5 {
			want = "queued"
		}
		if g.State != want {
			t.Fatalf("project %d: %s", n, g.State)
		}
		services = append(services, s)
		runners = append(runners, runner)
	}
	// A valid result alone does not release a still-running native execution.
	runners[0].finish(`{"tests":["unit.rules"]}`)
	runners[0].run.StopStatus = "running"
	if err := services[0].Observe(ctx); err != nil {
		t.Fatal(err)
	}
	if err := services[5].Observe(ctx); err != nil {
		t.Fatal(err)
	}
	if runners[5].submits != 0 {
		t.Fatal("released before stopped")
	}
	runners[0].run.StopStatus = "stopped"
	runners[0].run.Revision++
	if err := services[0].Observe(ctx); err != nil {
		t.Fatal(err)
	}
	// Observing a later project first cannot jump the queue.
	if err := services[6].Observe(ctx); err != nil {
		t.Fatal(err)
	}
	if runners[6].submits != 0 {
		t.Fatal("FIFO bypass")
	}
	if err := services[5].Observe(ctx); err != nil {
		t.Fatal(err)
	}
	if runners[5].submits != 1 {
		t.Fatal("available slot not consumed")
	}
	if err := services[6].Observe(ctx); err != nil {
		t.Fatal(err)
	}
	if runners[6].submits != 0 {
		t.Fatal("exceeded shared limit")
	}
}

func TestGenerationQueueCancelAndRestartUnknown(t *testing.T) {
	ctx := context.Background()
	s, store, runner, in := generationFixture(t)
	s.Queue = NewGenerationQueue(1)
	runner.lost, runner.hidden = true, true
	if _, err := s.Generate(ctx, in); err == nil {
		t.Fatal("expected unknown")
	}
	other, _, otherRunner, cmd := generationFixture(t)
	other.Project, other.Queue = "other-project", s.Queue
	g, err := other.Generate(ctx, cmd)
	if err != nil || g.State != "queued" {
		t.Fatal(g, err)
	}
	if _, err = other.Stop(ctx, g.ID, strings.Repeat("b", 32)); err != nil {
		t.Fatal(err)
	}
	if err = other.Observe(ctx); err != nil {
		t.Fatal(err)
	}
	if otherRunner.submits != 0 || otherRunner.stops != 0 {
		t.Fatal("cancelled queue touched Runner")
	}
	// Reconstruct occupancy before any project can admit new work.
	restored := NewGenerationQueue(1)
	rows, _ := store.Generations(ctx, s.Project)
	for _, row := range rows {
		restored.Update(row)
	}
	s.Queue = restored
	third, _, thirdRunner, cmd := generationFixture(t)
	third.Project, third.Queue = "third-project", restored
	g, err = third.Generate(ctx, cmd)
	if err != nil || g.State != "queued" {
		t.Fatal(g, err)
	}
	if err = s.Observe(ctx); err != nil {
		t.Fatal(err)
	}
	if runner.submits != 1 || thirdRunner.submits != 0 {
		t.Fatal("unknown was resubmitted or failed to retain slot")
	}
	runner.hidden = false
	runner.finish(`{"tests":["unit.rules"]}`)
	if err = s.Observe(ctx); err != nil {
		t.Fatal(err)
	}
	if err = third.Observe(ctx); err != nil {
		t.Fatal(err)
	}
	if thirdRunner.submits != 1 {
		t.Fatal("settled original did not release slot")
	}
}

func TestGenerationQueueResumeOnlyUnsubmittedOutbox(t *testing.T) {
	ctx := context.Background()
	s, store, runner, in := generationFixture(t)
	s.Queue = NewGenerationQueue(1)
	// A different project's original unknown request occupies the sole slot.
	blocker := Generation{Project: "blocked", ID: "original", State: "unknown"}
	s.Queue.Update(blocker)
	g, err := s.Generate(ctx, in)
	if err != nil || !g.Unsubmitted() || runner.submits != 0 {
		t.Fatal(g, err)
	}
	s.Queue = NewGenerationQueue(1)
	if err = s.Observe(ctx); err != nil {
		t.Fatal(err)
	}
	if runner.submits != 1 {
		t.Fatal("pending outbox not dispatched after restart")
	}
	first, _ := store.Generation(ctx, s.Project, in.RequestID)
	if first.Unsubmitted() || first.Request.RequestID != g.Request.RequestID {
		t.Fatal("changed original request")
	}
	// A crash after persisting the ambiguity boundary but before receiving a
	// receipt must not submit again, even when Runner cannot find the request.
	first.State, first.RunID, first.InputDigest, first.Projection = "unknown", "", "", nil
	first.Revision++
	if err = store.SaveGeneration(ctx, first, first.Revision-1); err != nil {
		t.Fatal(err)
	}
	runner.hidden = true
	s.Queue = NewGenerationQueue(1)
	if err = s.Observe(ctx); err != nil {
		t.Fatal(err)
	}
	if runner.submits != 1 {
		t.Fatal("unknown boundary replayed")
	}
}

func TestGenerationQueuePersistenceWindowsAndBuildChange(t *testing.T) {
	for _, state := range []string{"unknown", "accepted"} {
		t.Run(state, func(t *testing.T) {
			s, store, runner, in := generationFixture(t)
			ctx := context.Background()
			store.denyState = state
			if _, err := s.Generate(ctx, in); err == nil {
				t.Fatal("expected failed save window")
			}
			g, err := store.Generation(ctx, s.Project, in.RequestID)
			if err != nil {
				t.Fatal(err)
			}
			if state == "unknown" && (!g.Unsubmitted() || runner.submits != 0) {
				t.Fatal("unsaved ambiguity boundary submitted", g)
			}
			if state == "accepted" && (g.Unsubmitted() || runner.submits != 1) {
				t.Fatal("lost receipt did not retain started intent", g)
			}
			store.denyState = ""
			if err = s.Observe(ctx); err != nil {
				t.Fatal(err)
			}
			if runner.submits != 1 {
				t.Fatal("lost window caused duplicate submission", runner.submits)
			}
		})
	}
	s, _, runner, in := generationFixture(t)
	s.Queue = NewGenerationQueue(1)
	s.Queue.Update(Generation{Project: "busy", ID: "original", State: "unknown"})
	if _, err := s.Generate(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	s.Build.Version = "new-build"
	if err := s.Observe(context.Background()); err != nil {
		t.Fatal(err)
	}
	g, _ := s.Store.Generation(context.Background(), s.Project, in.RequestID)
	if g.State != "rejected" || runner.submits != 0 {
		t.Fatal("unsent old build request rebound to a new execution", g)
	}
}
