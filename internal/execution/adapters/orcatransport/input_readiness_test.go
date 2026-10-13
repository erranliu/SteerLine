package orcatransport

import (
	"context"
	"errors"
	"strings"
	"testing"

	native "github.com/erranliu/SteerLine/internal/execution/adapters/protocol/orca"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestOrcaInputReadiness(t *testing.T) {
	for _, scenario := range []string{"ready", "slow-start", "working", "waiting", "blocked", "missing", "wrong-pane", "query-failure"} {
		t.Run(scenario, func(t *testing.T) {
			l, f, s := fixture(t)
			l.profile.WaitTimeout = "1ms"
			r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
			if err != nil {
				t.Fatal(err)
			}
			bind(&s, r)
			base := fixtureState(10, "previous", "done")
			current := base
			switch scenario {
			case "working", "waiting", "blocked":
				current = fixtureState(10, "previous", scenario)
			case "missing":
				current.MainAgent = nil
			case "wrong-pane":
				current.PaneKey = "another:pane"
			}
			feed := &controlledStateFeed{initial: base, current: current}
			if scenario == "slow-start" {
				l.profile.WaitTimeout = "2s"
				feed.queryStates = []native.AgentState{fixtureState(11, "initializing", "working"), fixtureState(12, "initializing", "done")}
			}
			if scenario == "query-failure" {
				feed.err = errors.New("unavailable raw state")
			}
			l.stateFactory = func(context.Context, string, b.ProcessIdentity) (native.StateFeed, error) { return feed, nil }
			input := b.TurnInput{CommandID: strings.Repeat("c", 32), Prompt: "Read only"}
			s.PendingTurn = &input
			r, _, err = l.Execute(context.Background(), request(s, b.LifecyclePrepareSession, b.LifecycleInput{Turn: &input}))
			if scenario == "ready" || scenario == "slow-start" {
				if err != nil || r.Disposition != b.OperationConfirmed {
					t.Fatal(r, err)
				}
				if scenario == "slow-start" && feed.queries != 2 {
					t.Fatal("initialization was not awaited", feed.queries)
				}
			} else if err == nil || r.Disposition != b.OperationRejected {
				t.Fatal("unready raw state accepted", r, err)
			}
			if f.nativeSends != 0 {
				t.Fatal("task input sent during readiness")
			}
			for _, call := range f.calls {
				if call[0] == "terminal" && (call[1] == "read" || call[1] == "wait") {
					t.Fatal("readiness depended on the screen", call)
				}
			}
		})
	}
}

func TestOrcaInputRejectedReceipt(t *testing.T) {
	t.Run("bound", testBoundInputRejection)
	for _, scenario := range []string{"wrong-request", "generic-error"} {
		t.Run(scenario, func(t *testing.T) {
			l, f, s := fixture(t)
			r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
			if err != nil {
				t.Fatal(err)
			}
			bind(&s, r)
			s, input := prepared(t, l, s, strings.Repeat("c", 32))
			f.rejectInput, f.rejectWrongRequest = true, scenario == "wrong-request"
			f.rejectUnavailable = scenario == "generic-error"
			r, observer, err := l.Execute(context.Background(), request(s, b.LifecycleStartTurn, b.LifecycleInput{Turn: &input}))
			if err == nil || observer != nil || r.Disposition != b.OperationUnknown {
				t.Fatal("unbound error treated as a safe rejection", r, err)
			}
			bind(&s, r)
			before := len(f.calls)
			_, _, _ = l.Reconcile(context.Background(), request(s, b.LifecycleStartTurn, b.LifecycleInput{Turn: &input}))
			for _, call := range f.calls[before:] {
				if call[0] == "terminal" && call[1] == "send" {
					t.Fatal("unknown input resent")
				}
			}
		})
	}
}

func testBoundInputRejection(t *testing.T) {
	l, f, s := fixture(t)
	r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
	if err != nil {
		t.Fatal(err)
	}
	bind(&s, r)
	s, input := prepared(t, l, s, strings.Repeat("c", 32))
	f.rejectInput = true
	req := request(s, b.LifecycleStartTurn, b.LifecycleInput{Turn: &input})
	r, observer, err := l.Execute(context.Background(), req)
	if err == nil || observer != nil || r.Disposition != b.OperationRejected {
		t.Fatal(r, err)
	}
	bind(&s, r)
	req.Snapshot = s
	before := len(f.calls)
	r, _, err = l.Reconcile(context.Background(), req)
	if err != nil || r.Disposition != b.OperationRejected || len(f.calls) != before || f.nativeSends != 0 {
		t.Fatal("rejection reconciled as unknown or resent", r, err)
	}
}
