package orcatransport

import (
	"context"
	"errors"
	"strings"
	"testing"

	native "github.com/erranliu/SteerLine/internal/execution/adapters/protocol/orca"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

type recoveryStateFeed struct {
	initial, current native.AgentState
	err              error
	closed           bool
}

func TestRecoveryRejectsProviderReplacementInsideOriginalTerminal(t *testing.T) {
	l, _, s := fixture(t)
	r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
	if err != nil {
		t.Fatal(err)
	}
	bind(&s, r)
	state, err := l.state(s)
	if err != nil {
		t.Fatal(err)
	}
	s.Session.CurrentCommandID = strings.Repeat("c", 32)
	state.Turns = []Turn{{Command: s.Session.CurrentCommandID, Prompt: "original", InputDigest: digest([]byte("original")), Attempted: true, Accepted: true, Started: true,
		Native: NativeProof{Pane: "tab:pane", Provider: "original-provider", Authority: "hooks", Revision: 1, WorkingAt: 1, RootWorkingAt: 1}}}
	s.Implementation, err = l.envelope(s, state)
	if err != nil {
		t.Fatal(err)
	}
	current := fixtureState(2, "original", "done")
	current.MainAgent.Outcome = "success"
	current.ProviderSession.ID = "replacement-provider"
	feed := &recoveryStateFeed{initial: current, current: current}
	l.stateFactory = func(context.Context, string, b.ProcessIdentity) (native.StateFeed, error) { return feed, nil }
	evidence, err := l.VerifyManualRecovery(context.Background(), s)
	if err != nil || evidence.SessionIdle || !feed.closed {
		t.Fatalf("same terminal hid provider replacement: %+v %v", evidence, err)
	}
	current.ProviderSession.ID, current.PaneKey = "original-provider", "replacement:pane"
	feed = &recoveryStateFeed{initial: current, current: current}
	evidence, err = l.VerifyManualRecovery(context.Background(), s)
	if err != nil || evidence.SessionIdle || !feed.closed {
		t.Fatalf("same terminal hid pane replacement: %+v %v", evidence, err)
	}
}

func (f *recoveryStateFeed) Initial() native.AgentState          { return f.initial }
func (f *recoveryStateFeed) Drain() ([]native.AgentState, error) { return nil, nil }
func (f *recoveryStateFeed) Query(context.Context) (native.AgentState, error) {
	return f.current, f.err
}
func (f *recoveryStateFeed) Close() { f.closed = true }

func TestRecoveryRejectsStaleScreenIdleAgainstNativeMainAgent(t *testing.T) {
	for _, fault := range []string{"working", "waiting", "invalid_outcome", "cancellation", "interrupted", "missing_agent", "pane", "provider", "stale_time", "stale_done", "query_error"} {
		t.Run(fault, func(t *testing.T) {
			l, _, s := fixture(t)
			r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
			if err != nil {
				t.Fatal(err)
			}
			bind(&s, r)
			initial := fixtureState(2, "", "done")
			current := fixtureState(2, "", "done")
			current.MainAgent.Outcome = "success"
			feed := &recoveryStateFeed{initial: initial, current: current}
			switch fault {
			case "working", "waiting":
				feed.current.MainAgent.State = fault
			case "invalid_outcome":
				feed.current.MainAgent.Outcome = "failure"
			case "cancellation":
				feed.current.MainAgent.Outcome = "cancellation"
			case "interrupted":
				feed.current.Interrupted = true
			case "missing_agent":
				feed.current.MainAgent = nil
			case "pane":
				feed.current.PaneKey = "other:pane"
			case "provider":
				feed.current.ProviderSession.ID = "other-session"
			case "stale_time":
				feed.current.UpdatedAt = feed.current.MainAgent.StateStartedAt - 1
			case "stale_done":
				feed.current.MainAgent.StateStartedAt--
			case "query_error":
				feed.err = errors.New("native query disconnected")
			}
			l.stateFactory = func(context.Context, string, b.ProcessIdentity) (native.StateFeed, error) { return feed, nil }
			evidence, err := l.VerifyManualRecovery(context.Background(), s)
			if evidence.SessionIdle || !feed.closed || (err != nil) != (fault == "query_error") {
				t.Fatalf("stale screen authorized input: %+v %v; closed=%v", evidence, err, feed.closed)
			}
		})
	}
}
