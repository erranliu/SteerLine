package orcatransport

import (
	"context"
	"strings"
	"testing"
	"time"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestOrcaInputReadiness(t *testing.T) {
	for _, scenario := range []string{"slow_start", "timeout", "interactive"} {
		t.Run(scenario, func(t *testing.T) {
			l, f, s := fixture(t)
			l.profile.WaitTimeout = "30s"
			r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
			if err != nil {
				t.Fatal(err)
			}
			bind(&s, r)
			if scenario == "slow_start" {
				f.waitFailures = 1
			}
			if scenario == "timeout" {
				f.waitFailures = 3
			}
			if scenario == "interactive" {
				f.waitBlocked = "codex-trust-workspace"
			}
			input := b.TurnInput{CommandID: strings.Repeat("c", 32), Prompt: "Read only"}
			s.PendingTurn = &input
			r, _, err = l.Execute(context.Background(), request(s, b.LifecyclePrepareSession, b.LifecycleInput{Turn: &input}))
			if scenario == "slow_start" {
				if err != nil || r.Disposition != b.OperationConfirmed || f.waitCalls != 2 {
					t.Fatal(r, err, f.waitCalls)
				}
			} else {
				want := 2
				if scenario == "interactive" {
					want = 1
				}
				if err == nil || r.Disposition != b.OperationRejected || f.waitCalls != want || !strings.Contains(err.Error(), "screen=") || !strings.Contains(err.Error(), "no input was sent") {
					t.Fatal(r, err, f.waitCalls)
				}
			}
			for _, budget := range f.waitBudgets {
				if budget < 40*time.Second {
					t.Fatal("wait budget truncated server timeout", budget)
				}
			}
			if f.nativeSends != 0 {
				t.Fatal("task input sent during readiness")
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
