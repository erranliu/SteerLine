package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

type messageCLI struct {
	runnerbridge.CLI
	call func(context.Context) (runnerclient.Response, error)
}

func (c messageCLI) Call(ctx context.Context, command string, args []string, _ *runnerclient.Request) (runnerclient.Response, error) {
	if command != "last-message" || len(args) != 2 || args[0] != "--run-id" {
		panic("unexpected public command")
	}
	return c.call(ctx)
}

func TestActivityMessageObservationFiltersToolsAndLimitsEveryAttempt(t *testing.T) {
	now := time.Now().UTC()
	o := newMessageObserver()
	o.register(context.Background(), "p", nil)
	o.now = func() time.Time { return now }
	c := runnerstate.Candidate{Project: "p", ID: "c", RunID: "run", Projection: &runnercontract.Run{Status: "running", Execution: runnercontract.Execution{AttemptID: "attempt", SessionID: "session"}}}
	calls := 0
	kind := "assistant"
	var callErr error
	cli := messageCLI{call: func(ctx context.Context) (runnerclient.Response, error) {
		calls++
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > time.Second {
			t.Error("unbounded public read")
		}
		if callErr != nil {
			return runnerclient.Response{}, callErr
		}
		raw, _ := json.Marshal(api.LastMessageView{RunID: "run", AttemptID: "attempt", Message: &api.LastMessage{RunID: "run", AttemptID: "attempt", SessionID: "session", TurnID: "turn", Kind: kind, Text: kind, ObservedAt: now}})
		return runnerclient.Response{Data: raw}, nil
	}}
	first, err := o.observe(context.Background(), c, cli, nil, false)
	if err != nil || first.Message == nil {
		t.Fatalf("assistant: %+v %v", first, err)
	}
	at := first.Message.ObservedAt
	now = now.Add(3 * time.Second)
	kind = "tool"
	tool, err := o.observe(context.Background(), c, cli, nil, false)
	if err != nil || tool.Message.Text != "assistant" || !tool.Message.ObservedAt.Equal(at) {
		t.Fatalf("tool replaced readable observation: %+v %v", tool, err)
	}
	callErr = errors.New("offline")
	now = now.Add(3 * time.Second)
	_, err = o.observe(context.Background(), c, cli, nil, false)
	if err == nil {
		t.Fatal("missing read diagnostic")
	}
	before := calls
	_, _ = o.observe(context.Background(), c, cli, nil, true)
	if calls != before {
		t.Fatal("failed foreground read bypassed background interval")
	}
	now = now.Add(10 * time.Second)
	_, _ = o.observe(context.Background(), c, cli, nil, true)
	o.mu.Lock()
	done := o.entries[messageKey(c)].running
	o.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("background read did not settle")
		}
	}
	if calls != before+1 {
		t.Fatalf("expected one bounded background read, got %d", calls)
	}
	c.Projection = &runnercontract.Run{Status: "running", Execution: runnercontract.Execution{AttemptID: "new", SessionID: "new-session"}}
	callErr = nil
	if view, err := o.observe(context.Background(), c, cli, nil, false); err == nil || view.Message != nil {
		t.Fatalf("old binding accepted: %+v %v", view, err)
	}
}

func TestActivityMessageObservationDoesNotBlockCallerAndSharesSlots(t *testing.T) {
	o := newMessageObserver()
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	defer close(release)
	cli := messageCLI{call: func(ctx context.Context) (runnerclient.Response, error) {
		entered <- struct{}{}
		select {
		case <-ctx.Done():
			return runnerclient.Response{}, ctx.Err()
		case <-release:
			return runnerclient.Response{}, errors.New("released")
		}
	}}
	c := runnerstate.Candidate{Project: "p", ID: "c", RunID: "run", Projection: &runnercontract.Run{Status: "running", Execution: runnercontract.Execution{AttemptID: "attempt", SessionID: "session"}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	o.register(ctx, "p", nil)
	start := time.Now()
	_, _ = o.observe(ctx, c, cli, nil, true)
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("background read blocked owner")
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("read not started")
	}
	_, _ = o.observe(ctx, c, cli, nil, true)
	if len(entered) != 0 {
		t.Fatal("same binding read twice")
	}
	c.ID = "second"
	_, _ = o.observe(ctx, c, cli, nil, true)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("second slot absent")
	}
	c.ID = "third"
	_, _ = o.observe(ctx, c, cli, nil, true)
	o.mu.Lock()
	done := o.entries[messageKey(c)].running
	o.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("busy background observation queued")
		}
	}
	if len(o.slots) != 2 {
		t.Fatal("global public read concurrency changed")
	}
	cancel()
}

func TestActivityMessageObservationOutlivesTickAndStopsWithProject(t *testing.T) {
	project, cancel := context.WithCancel(context.Background())
	defer cancel()
	o := newMessageObserver()
	o.register(project, "p", nil)
	c := runnerstate.Candidate{Project: "p", ID: "c", RunID: "run", Projection: &runnercontract.Run{Status: "running", Execution: runnercontract.Execution{AttemptID: "attempt", SessionID: "session"}}}
	entered := make(chan context.Context, 1)
	release := make(chan struct{})
	cli := messageCLI{call: func(ctx context.Context) (runnerclient.Response, error) {
		entered <- ctx
		select {
		case <-ctx.Done():
			return runnerclient.Response{}, ctx.Err()
		case <-release:
			return runnerclient.Response{Data: json.RawMessage(`{"run_id":"run","attempt_id":"attempt","message":{"run_id":"run","attempt_id":"attempt","session_id":"session","turn_id":"turn","kind":"assistant","text":"sample","observed_at":"2026-11-10T00:00:00Z"}}`)}, nil
		}
	}}
	actor := &taskrunner.Actor{}
	var tick context.Context
	if err := actor.RunWork(project, func(work context.Context) error {
		tick = work
		_, err := o.observe(work, c, cli, nil, true)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	read := <-entered
	if tick.Err() == nil || read.Err() != nil {
		t.Fatal("public read inherited completed tick context")
	}
	close(release)
	view, err := o.observe(project, c, cli, nil, false)
	if err != nil || view.Message == nil || view.Message.Text != "sample" {
		t.Fatalf("sample lost after tick: %+v %v", view, err)
	}
	c.ID = "second"
	release = make(chan struct{})
	_, _ = o.observe(project, c, cli, nil, true)
	read = <-entered
	cancel()
	o.release("p")
	if read.Err() == nil || len(o.slots) != 0 {
		t.Fatal("project shutdown returned before background read settled")
	}
}
