package orcatransport

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	native "github.com/erranliu/SteerLine/internal/execution/adapters/protocol/orca"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

type disconnectedStateFeed struct{ *controlledStateFeed }

func (f *disconnectedStateFeed) Drain() ([]native.AgentState, error) {
	return nil, errors.New("notification connection closed")
}

type countedStateFeed struct {
	*controlledStateFeed
	closes int
}

func (f *countedStateFeed) Close() { f.closes++ }

func TestOrcaCompletionQueryRecoveryClosesFailedRead(t *testing.T) {
	l, host, s := fixture(t)
	r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
	if err != nil {
		t.Fatal(err)
	}
	bind(&s, r)
	now := time.Unix(100, 0).UTC()
	l.clock = func() time.Time { return now }
	base := fixtureState(10, "previous", "done")
	feed := &countedStateFeed{controlledStateFeed: &controlledStateFeed{initial: base, current: base}}
	l.stateFactory = func(context.Context, string, b.ProcessIdentity) (native.StateFeed, error) { return feed, nil }
	s, input := prepared(t, l, s, strings.Repeat("c", 32))
	r, err = l.submit(context.Background(), request(s, b.LifecycleStartTurn, b.LifecycleInput{Turn: &input}))
	if err != nil {
		t.Fatal(err)
	}
	bind(&s, r)
	l.closeStateFeed()
	feed.closes = 0
	now = now.Add(30 * time.Second)
	l.queryNative = func(context.Context, b.ProcessIdentity, string) (native.AgentState, error) {
		// Identity changes between opening the restored feed and the final CLI
		// check. No observer is returned to own and release that new feed.
		host.incarnation = "replacement"
		end := fixtureState(12, input.Prompt, "done")
		end.LastAssistantMessage = "complete reply"
		return end, nil
	}
	_, observer, err := l.ObserveLifecycle(context.Background(), s)
	if err == nil || observer != nil || feed.closes != 1 || l.feed != nil {
		t.Fatal("failed recovery read leaked its subscription", err, feed.closes)
	}
	if host.nativeSends != 1 {
		t.Fatal("failed recovery replayed its input")
	}
}

func TestOrcaCompletionQuery(t *testing.T) {
	for _, scenario := range []string{"quiet-done", "old-done", "missing-baseline", "stale-snapshot", "accepted-only", "wrong-prompt", "pane", "provider", "session-boundary", "restored", "tool-output", "interrupted", "failed", "truncated", "missing-body", "query-failure", "missing-agent", "missing-provider", "query-other-input", "query-session-boundary", "subscription-failure", "open-failure", "identity-conflict", "open-identity-conflict", "other-input", "other-input-then-waiting", "other-input-without-working", "session-boundary-working", "restore"} {
		t.Run(scenario, func(t *testing.T) {
			l, host, s := fixture(t)
			r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
			if err != nil {
				t.Fatal(err)
			}
			bind(&s, r)
			now := time.Unix(100, 0).UTC()
			l.clock = func() time.Time { return now }
			base := fixtureState(10, "previous", "done")
			if scenario == "missing-baseline" {
				base.MainAgent.StateStartedAt = 0
			}
			base.LastAssistantMessage = "same complete reply"
			feed := &controlledStateFeed{initial: base, current: base}
			l.stateFactory = func(context.Context, string, b.ProcessIdentity) (native.StateFeed, error) { return feed, nil }
			s, input := prepared(t, l, s, strings.Repeat("c", 32))
			host.unstarted = scenario == "accepted-only"
			r, err = l.submit(context.Background(), request(s, b.LifecycleStartTurn, b.LifecycleInput{Turn: &input}))
			if err != nil {
				t.Fatal(err)
			}
			bind(&s, r)
			feed.queries = 0
			end := fixtureState(12, input.Prompt, "done")
			end.Observation = nil // Actual worktree.ps snapshots omit it.
			end.LastAssistantMessage = base.LastAssistantMessage
			feed.current = end
			switch scenario {
			case "old-done":
				feed.current = base
			case "stale-snapshot":
				feed.current.UpdatedAt = end.MainAgent.StateStartedAt - 1
			case "wrong-prompt":
				feed.current.Prompt = "unrelated input"
			case "pane":
				feed.current.PaneKey = "other:pane"
			case "provider":
				feed.current.ProviderSession.ID = "replacement"
			case "session-boundary":
				feed.current.SessionBoundary = true
			case "restored":
				feed.current.RestoredUnconfirmed = true
			case "tool-output":
				feed.current.ToolOutput = true
			case "interrupted":
				feed.current.Interrupted = true
			case "failed":
				root := *feed.current.MainAgent
				root.Outcome = "failure"
				feed.current.MainAgent = &root
			case "truncated":
				feed.current.LastAssistantMessage = strings.Repeat("x", 8000)
			case "missing-body":
				feed.current.LastAssistantMessage = ""
			case "query-failure":
				feed.err = errors.New("temporary query failure")
			case "missing-agent":
				_, feed.err = native.DecodeOriginalAgent([]byte(`{"worktrees":[]}`), s.Ref.External.WorktreeID, end.PaneKey)
			case "missing-provider":
				feed.current.ProviderSession.ID = ""
			case "query-other-input", "query-session-boundary":
				feed.current = fixtureState(11, "unrelated", "working")
				feed.current.Observation = nil
				if scenario == "query-session-boundary" {
					feed.current.Prompt = preview(input.Prompt)
					feed.current.SessionBoundary = true
				}
				l.feed = &disconnectedStateFeed{feed}
			case "subscription-failure":
				l.feed = &disconnectedStateFeed{feed}
			case "open-failure":
				l.closeStateFeed()
				l.stateFactory = func(context.Context, string, b.ProcessIdentity) (native.StateFeed, error) {
					return nil, errors.New("subscription unavailable")
				}
				l.queryNative = func(context.Context, b.ProcessIdentity, string) (native.AgentState, error) {
					feed.queries++
					return feed.current, nil
				}
			case "identity-conflict":
				l.queryNative = func(context.Context, b.ProcessIdentity, string) (native.AgentState, error) {
					return native.AgentState{}, native.ErrStateIdentityChanged
				}
			case "open-identity-conflict":
				l.closeStateFeed()
				l.stateFactory = func(context.Context, string, b.ProcessIdentity) (native.StateFeed, error) {
					return nil, native.ErrStateIdentityChanged
				}
				l.queryNative = func(context.Context, b.ProcessIdentity, string) (native.AgentState, error) { return end, nil }
			case "other-input", "other-input-then-waiting":
				start := fixtureState(11, input.Prompt, "working")
				other := fixtureState(12, "unrelated", "working")
				other.Observation.Boundary = true
				feed.events = []native.AgentState{start, other}
				if scenario == "other-input-then-waiting" {
					feed.events = append(feed.events, fixtureState(13, input.Prompt, "waiting"))
				}
			case "other-input-without-working", "session-boundary-working":
				other := fixtureState(11, "unrelated", "working")
				if scenario == "session-boundary-working" {
					other.Prompt = preview(input.Prompt)
					other.SessionBoundary = true
				}
				feed.events = []native.AgentState{other}
			case "restore":
				// Reconstruct the adapter from the saved resource envelope; no live
				// notification or working proof survives this observer restart.
				l.closeStateFeed()
				port, err := NewLifecycleWithStateFactory(l.setup, l.stateFactory)
				if err != nil {
					t.Fatal(err)
				}
				l = port.(*Lifecycle)
				l.clock = func() time.Time { return now }
			}
			poll := func() b.LifecycleReceipt {
				r, _, err := l.observeOnce(context.Background(), s)
				if err != nil {
					t.Fatal(err)
				}
				bind(&s, r)
				return r
			}
			if poll().TurnEvidence != nil || feed.queries != 0 {
				t.Fatal("queried before interval or accepted old reply")
			}
			now = now.Add(30 * time.Second)
			r = poll()
			positive := scenario == "quiet-done" || scenario == "subscription-failure" || scenario == "open-failure" || scenario == "restore"
			if (r.TurnEvidence != nil) != positive {
				t.Fatalf("completion verdict %s: %+v", scenario, r)
			}
			if scenario == "missing-body" || scenario == "query-failure" || scenario == "old-done" || scenario == "missing-agent" || scenario == "missing-provider" {
				if poll().TurnEvidence != nil || feed.queries != 1 {
					t.Fatal("unavailable result caused a tight query loop")
				}
				feed.current, feed.err = end, nil
				now = now.Add(30 * time.Second)
				r = poll()
				if r.TurnEvidence == nil || feed.queries != 2 {
					t.Fatal("original query did not recover", r.Diagnostic, feed.queries)
				}
				positive = true
			}
			if positive {
				state, err := l.state(s)
				if err != nil || turnFor(&state, input.CommandID).Native.WorkingAt != 0 || r.TurnEvidence.LastMessage != end.LastAssistantMessage || r.TurnEvidence.NativeOutcome != "unknown" {
					t.Fatal("completion invented working, changed body or outcome", err)
				}
				queries := feed.queries
				if again := poll(); again.TurnEvidence == nil || feed.queries != queries {
					t.Fatal("settled result was queried again")
				}
			}
			if scenario == "identity-conflict" || scenario == "open-identity-conflict" || scenario == "other-input-then-waiting" || scenario == "other-input-without-working" || scenario == "session-boundary-working" || scenario == "query-other-input" || scenario == "query-session-boundary" {
				feed.current = end
				l.stateFactory = func(context.Context, string, b.ProcessIdentity) (native.StateFeed, error) { return feed, nil }
				l.closeStateFeed()
				port, err := NewLifecycleWithStateFactory(l.setup, l.stateFactory)
				if err != nil {
					t.Fatal(err)
				}
				l = port.(*Lifecycle)
				l.clock = func() time.Time { return now }
				l.queryNative = func(context.Context, b.ProcessIdentity, string) (native.AgentState, error) { return end, nil }
				now = now.Add(30 * time.Second)
				if poll().TurnEvidence != nil {
					t.Fatal("retry erased original identity conflict")
				}
			}
			if host.nativeSends != 1 {
				t.Fatal("observation resent original input")
			}
		})
	}
}
