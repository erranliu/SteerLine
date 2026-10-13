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

type controlledStateFeed struct {
	initial, current native.AgentState
	events           []native.AgentState
	queries          int
	err              error
}

func (f *controlledStateFeed) Initial() native.AgentState { return f.initial }
func (f *controlledStateFeed) Drain() ([]native.AgentState, error) {
	events := f.events
	f.events = nil
	return events, nil
}
func (f *controlledStateFeed) Query(context.Context) (native.AgentState, error) {
	f.queries++
	return f.current, f.err
}
func (*controlledStateFeed) Close() {}

func TestOrcaNativeStateGate(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	turn := Turn{Accepted: true, Started: true, Prompt: strings.Repeat("长输入 ", 100)}
	base := fixtureState(10, "previous", "done")
	turn.Native = nativeBaseline(base, now)
	start := fixtureState(11, turn.Prompt, "working")
	turn.Native.signal(start, &turn, now)
	if turn.Native.WorkingAt == 0 || turn.Native.EndedAt != 0 {
		t.Fatal("long input was not bound to its native start")
	}
	if turn.Native.wantsQuery(now.Add(29 * time.Second)) {
		t.Fatal("queried before silence deadline")
	}
	if !turn.Native.wantsQuery(now.Add(30 * time.Second)) {
		t.Fatal("missing silence query")
	}
	turn.Native.Queried = true
	if turn.Native.query(start, now.Add(30*time.Second)) || turn.Native.wantsQuery(now.Add(60*time.Second)) {
		t.Fatal("working or same silent window completed/requeried")
	}
	hint := fixtureState(12, turn.Prompt, "done")
	hint.Observation = nil
	hint.MainAgent = nil
	turn.Native.signal(hint, &turn, now.Add(61*time.Second))
	if turn.Native.EndedAt != 0 || !turn.Native.wantsQuery(now.Add(61*time.Second)) {
		t.Fatal("new title done did not trigger confirmation only")
	}
	turn.Native.Queried = true
	turn.Native.HintPending = false
	turn.Native.signal(hint, &turn, now.Add(62*time.Second))
	if turn.Native.wantsQuery(now.Add(62 * time.Second)) {
		t.Fatal("duplicate hint repeated query")
	}
	old := base
	if turn.Native.query(old, now.Add(62*time.Second)) {
		t.Fatal("old done completed the new turn")
	}
	end := fixtureState(12, turn.Prompt, "done")
	end.Observation = nil
	end.LastAssistantMessage = `{"summary":"本轮结果","url":"https://example.com/pull/1"}`
	if !turn.Native.query(end, now.Add(63*time.Second)) || turn.Native.Outcome != "unknown" || turn.Native.ExpectedReply == "" {
		t.Fatal("fresh native root end was not accepted independently of outcome")
	}
	if turn.Native.ExpectedReply != replyAssociation("{\"summary\":\"本轮\n结果\",\"url\":\"https://example.com/\npull/1\"}") {
		t.Fatal("display wrapping changed current reply association")
	}
	for _, kind := range []string{"waiting", "child", "cancelled", "other-provider", "other-pane"} {
		t.Run(kind, func(t *testing.T) {
			p := nativeBaseline(base, now)
			p.signal(start, &turn, now)
			s := end
			root := *end.MainAgent
			s.MainAgent = &root
			switch kind {
			case "waiting":
				s.MainAgent.State = "waiting"
			case "child":
				s.MainAgent.State = "working"
			case "cancelled":
				s.MainAgent.Outcome = "cancellation"
			case "other-provider":
				s.ProviderSession.ID = "other"
			case "other-pane":
				s.PaneKey = "other"
			}
			if p.query(s, now.Add(time.Minute)) || p.EndedAt != 0 {
				t.Fatal("unverified main-agent end accepted")
			}
		})
	}
	cut := nativeBaseline(base, now)
	cut.signal(start, &turn, now)
	cutEnd := end
	cutEnd.LastAssistantMessage = strings.Repeat("x", 8000)
	if !cut.query(cutEnd, now.Add(time.Minute)) || cut.ExpectedReply != "" {
		t.Fatal("clipped native message treated as complete")
	}
}

func TestOrcaNativeStateLifecycle(t *testing.T) {
	for _, environment := range []string{"", "paired-worker"} {
		t.Run(environment, func(t *testing.T) {
			l, f, s := fixtureOn(t, environment)
			r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
			if err != nil {
				t.Fatal(err)
			}
			bind(&s, r)
			base := fixtureState(10, "previous", "done")
			feed := &controlledStateFeed{initial: base, current: base}
			l.stateFactory = func(context.Context, string, b.ProcessIdentity) (native.StateFeed, error) { return feed, nil }
			s, input := prepared(t, l, s, strings.Repeat("c", 32))
			// Use the actual submission method without starting an asynchronous observer.
			r, err = l.submit(context.Background(), request(s, b.LifecycleStartTurn, b.LifecycleInput{Turn: &input}))
			if err != nil {
				t.Fatal(err)
			}
			bind(&s, r)
			instant := time.Unix(100, 0).UTC()
			l.clock = func() time.Time { return instant }
			start := fixtureState(11, input.Prompt, "working")
			feed.events = []native.AgentState{start}
			feed.current = start
			old := []string{"› previous", "• {\"summary\":\"old report\"}", "  Worked for 90s • 18:50", "› Ask Codex"}
			f.lines = old
			poll := func() b.LifecycleReceipt {
				r, _, err := l.observeOnce(context.Background(), s)
				if err != nil {
					t.Fatal(err)
				}
				bind(&s, r)
				return r
			}
			if poll().TurnEvidence != nil {
				t.Fatal("old screen ended native working")
			}
			instant = instant.Add(30 * time.Second)
			if poll().TurnEvidence != nil || feed.queries != 1 {
				t.Fatal("silence query did not retain working")
			}
			instant = instant.Add(30 * time.Second)
			poll()
			if feed.queries != 1 {
				t.Fatal("same silence window requeried")
			}
			hint := fixtureState(12, input.Prompt, "done")
			hint.Observation = nil
			hint.MainAgent = nil
			end := fixtureState(12, input.Prompt, "done")
			end.Observation = nil
			end.LastAssistantMessage = `{"summary":"current result"}`
			feed.events = []native.AgentState{hint}
			feed.current = end
			r = poll()
			if feed.queries != 2 || !r.State.TurnCompleted || r.State.WorkerLive || r.TurnEvidence != nil {
				t.Fatal("end/reply states conflated")
			}
			f.lines = []string{"› current", "• " + end.LastAssistantMessage, "  Worked for 2s • 18:50", "› Ask Codex"}
			r, _, err = settled(t, l, s)
			if err != nil || r.TurnEvidence == nil || r.TurnEvidence.LastMessage != end.LastAssistantMessage || r.TurnEvidence.NativeOutcome != "unknown" {
				t.Fatal("current complete ordinary reply did not settle", err)
			}
		})
	}
}

func TestOrcaNativeStateBinding(t *testing.T) {
	t.Run("markdown-rendering", func(t *testing.T) {
		raw := "Created [result.txt](C:/debug/result.txt) with `UTF-8` and **LF** endings."
		if replyAssociation(raw) != replyAssociation("Created result.txt with UTF-8 and LF endings.") {
			t.Fatal("native inline Markdown did not match its rendered ordinary reply")
		}
		p := NativeProof{}
		p.body(raw)
		if p.ReplyBody != raw {
			t.Fatal("association replaced the native Markdown original")
		}
		if !p.matchesVisible("Created result.txt (C:/debug/result.txt) with UTF-8 and LF endings.") || p.matchesVisible("Created result.txt (other.txt) with UTF-8 and LF endings.") {
			t.Fatal("visible link target was not bound to this native original")
		}
	})
	t.Run("native-original-is-immutable", func(t *testing.T) {
		p := NativeProof{}
		p.body(`{"summary":"a b"}`)
		p.body(`{"summary":"ab"}`)
		if p.Diagnostic == "" || p.ReplyBody != `{"summary":"a b"}` {
			t.Fatal("display association allowed a changed native original")
		}
	})
	l, f, s := fixture(t)
	r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
	if err != nil {
		t.Fatal(err)
	}
	bind(&s, r)
	base := fixtureState(10, "previous", "done")
	feed := &controlledStateFeed{initial: base, current: base, err: errors.New("bounded query failed")}
	l.stateFactory = func(context.Context, string, b.ProcessIdentity) (native.StateFeed, error) { return feed, nil }
	s, input := prepared(t, l, s, strings.Repeat("c", 32))
	r, err = l.submit(context.Background(), request(s, b.LifecycleStartTurn, b.LifecycleInput{Turn: &input}))
	if err != nil {
		t.Fatal(err)
	}
	bind(&s, r)
	start := fixtureState(11, input.Prompt, "working")
	hint := fixtureState(12, input.Prompt, "done")
	hint.MainAgent = nil
	hint.Observation = nil
	feed.events = []native.AgentState{start, hint}
	r, _, err = l.observeOnce(context.Background(), s)
	if err != nil || r.TurnEvidence != nil || r.Diagnostic == "" || feed.queries != 1 {
		t.Fatal("query failure completed a turn")
	}
	bind(&s, r)
	f.incarnation = "replacement"
	if _, _, err = l.observeOnce(context.Background(), s); err == nil {
		t.Fatal("replacement terminal was accepted")
	}
}
