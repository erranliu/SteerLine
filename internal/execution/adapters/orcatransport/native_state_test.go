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
	queryStates      []native.AgentState
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
	if len(f.queryStates) > 0 {
		f.current, f.queryStates = f.queryStates[0], f.queryStates[1:]
	}
	return f.current, f.err
}
func (*controlledStateFeed) Close() {}

func TestOrcaNativeStateGate(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	turn := Turn{Accepted: true, Started: true, Prompt: strings.Repeat("长输入 ", 100)}
	base := fixtureState(10, "previous", "done")
	turn.Native = nativeBaseline(base, now)
	start := fixtureState(11, turn.Prompt, "working")
	staleRoot := start
	root := *start.MainAgent
	root.StateStartedAt = base.MainAgent.StateStartedAt
	staleRoot.MainAgent = &root
	turn.Native.signal(staleRoot, &turn, now)
	if turn.Native.WorkingAt != 0 {
		t.Fatal("a new observation revision accepted the baseline root working clock")
	}
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
	turn.Native.LastSignal = now.Add(30 * time.Second)
	if turn.Native.query(start, now.Add(30*time.Second)) || turn.Native.wantsQuery(now.Add(59*time.Second)) || !turn.Native.wantsQuery(now.Add(60*time.Second)) {
		t.Fatal("working query completed or bounded retry was lost")
	}
	hint := fixtureState(12, turn.Prompt, "done")
	hint.Observation = nil
	hint.MainAgent = nil
	turn.Native.signal(hint, &turn, now.Add(61*time.Second))
	if turn.Native.EndedAt != 0 || !turn.Native.wantsQuery(now.Add(61*time.Second)) {
		t.Fatal("new title done did not trigger confirmation only")
	}
	turn.Native.HintPending = false
	turn.Native.LastSignal = now.Add(61 * time.Second)
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
	if !turn.Native.query(end, now.Add(63*time.Second)) || turn.Native.Outcome != "unknown" || turn.Native.ReplyDigest == "" {
		t.Fatal("fresh native root end was not accepted independently of outcome")
	}
	if turn.Native.ReplyDigest != digest([]byte(end.LastAssistantMessage)) {
		t.Fatal("native reply bytes were changed")
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
	if !cut.query(cutEnd, now.Add(time.Minute)) || cut.ReplyDigest != "" {
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
			feed.queries = 0
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
			if feed.queries != 2 {
				t.Fatal("bounded read-only retry missing")
			}
			hint := fixtureState(12, input.Prompt, "done")
			hint.Observation = nil
			hint.MainAgent = nil
			end := fixtureState(12, input.Prompt, "done")
			end.Observation = nil
			end.LastAssistantMessage = ""
			feed.events = []native.AgentState{hint}
			feed.current = end
			r = poll()
			if feed.queries != 3 || !r.State.TurnCompleted || r.State.WorkerLive || r.TurnEvidence != nil {
				t.Fatal("end/reply states conflated")
			}
			end.LastAssistantMessage = `{"summary":"current result"}`
			feed.current = end
			instant = instant.Add(30 * time.Second)
			f.lines = nil
			r, _, err = settled(t, l, s)
			if err != nil || r.TurnEvidence == nil || r.TurnEvidence.LastMessage != end.LastAssistantMessage || r.TurnEvidence.NativeOutcome != "unknown" {
				t.Fatal("current complete ordinary reply did not settle", err)
			}
		})
	}
}

func TestOrcaNativeStateBinding(t *testing.T) {
	t.Run("preserve-native-markdown", func(t *testing.T) {
		raw := "Created [result.txt](C:/debug/result.txt) with `UTF-8` and **LF** endings."
		p := NativeProof{}
		p.body(raw)
		if p.ReplyBody != raw || p.ReplyDigest != digest([]byte(raw)) {
			t.Fatal("native original changed")
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
	feed := &controlledStateFeed{initial: base, current: base}
	l.stateFactory = func(context.Context, string, b.ProcessIdentity) (native.StateFeed, error) { return feed, nil }
	s, input := prepared(t, l, s, strings.Repeat("c", 32))
	r, err = l.submit(context.Background(), request(s, b.LifecycleStartTurn, b.LifecycleInput{Turn: &input}))
	if err != nil {
		t.Fatal(err)
	}
	bind(&s, r)
	feed.err = errors.New("bounded query failed")
	feed.queries = 0
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

// Regressions from 101 (shortened link target) and 105 (offscreen boundaries).
// The native source and screen are intentionally independent in these fixtures.
func TestOrcaNativeFinalIgnoresScreen(t *testing.T) {
	for _, environment := range []string{"", "paired-worker"} {
		for _, scenario := range []string{"shortened-link", "missing-boundaries", "later-draft", "changed-screen"} {
			t.Run(environment+"/"+scenario, func(t *testing.T) {
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
				r, err = l.submit(context.Background(), request(s, b.LifecycleStartTurn, b.LifecycleInput{Turn: &input}))
				if err != nil {
					t.Fatal(err)
				}
				bind(&s, r)
				body := `{"outcome":"done","summary":"Published","outputs":{"pr_url":"https://github.com/erranliu/SteerlineTest/pull/229"}}`
				if scenario == "shortened-link" {
					body = "Updated [evidence/native-101.md](C:/work/evidence/native-101.md) with `UTF-8`."
					f.lines = []string{"› Implement.", "• Updated evidence/native-101.md (evidence/native-101.md) with UTF-8.", "  Worked for 1s • 13:51"}
				} else {
					f.lines = []string{"• A previous or partially visible response."}
				}
				if scenario == "later-draft" {
					f.draft = "a later user draft"
				}
				start := fixtureState(11, input.Prompt, "working")
				end := fixtureState(12, input.Prompt, "done")
				end.LastAssistantMessage = body
				feed.events, feed.current = []native.AgentState{start, end}, end
				before := len(f.calls)
				r, _, err = l.observeOnce(context.Background(), s)
				if err != nil || r.TurnEvidence == nil || !r.TurnEvidence.FinalVerified || r.TurnEvidence.LastMessage != body {
					t.Fatalf("native final was blocked by presentation: %v, %s", err, r.Diagnostic)
				}
				bind(&s, r)
				if scenario == "changed-screen" {
					f.lines = []string{"entirely different UI"}
				}
				replayed, _, err := l.observeOnce(context.Background(), s)
				if err != nil || replayed.TurnEvidence == nil || replayed.TurnEvidence.LastMessage != body {
					t.Fatal("settled native body was not retained", err)
				}
				for _, call := range f.calls[before:] {
					if len(call) >= 2 && call[0] == "terminal" && (call[1] == "read" || call[1] == "send") {
						t.Fatal("final collection read the screen or repeated input", call)
					}
				}
			})
		}
	}
}

func TestOrcaNativeFinalRetriesMissingBodyAndQueryFailure(t *testing.T) {
	for _, scenario := range []string{"missing-body", "query-failure"} {
		t.Run(scenario, func(t *testing.T) {
			l, _, s := fixture(t)
			r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
			if err != nil {
				t.Fatal(err)
			}
			bind(&s, r)
			base := fixtureState(10, "previous", "done")
			feed := &controlledStateFeed{initial: base, current: base}
			l.stateFactory = func(context.Context, string, b.ProcessIdentity) (native.StateFeed, error) { return feed, nil }
			s, input := prepared(t, l, s, strings.Repeat("c", 32))
			r, err = l.submit(context.Background(), request(s, b.LifecycleStartTurn, b.LifecycleInput{Turn: &input}))
			if err != nil {
				t.Fatal(err)
			}
			bind(&s, r)
			feed.queries = 0
			instant := time.Unix(100, 0).UTC()
			l.clock = func() time.Time { return instant }
			start := fixtureState(11, input.Prompt, "working")
			hint := fixtureState(12, input.Prompt, "done")
			hint.MainAgent, hint.Observation = nil, nil
			end := fixtureState(12, input.Prompt, "done")
			feed.events, feed.current = []native.AgentState{start, hint}, end
			if scenario == "query-failure" {
				feed.err = errors.New("temporary native query failure")
			}
			// Two unavailable reads must not permanently disable original-source polling.
			for i := 0; i < 2; i++ {
				r, _, err = l.observeOnce(context.Background(), s)
				if err != nil || r.TurnEvidence != nil || r.Diagnostic == "" {
					t.Fatal("unavailable read was accepted", err)
				}
				bind(&s, r)
				instant = instant.Add(30 * time.Second)
			}
			feed.err = nil
			end.LastAssistantMessage = "The complete original final arrived."
			feed.current = end
			r, _, err = l.observeOnce(context.Background(), s)
			if err != nil || feed.queries != 3 || r.TurnEvidence == nil || r.TurnEvidence.LastMessage != end.LastAssistantMessage {
				t.Fatal("original final did not recover", err, feed.queries)
			}
		})
	}
}

func TestOrcaObserverDeliversNativeFinalWithoutScreen(t *testing.T) {
	l, f, s := fixture(t)
	r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
	if err != nil {
		t.Fatal(err)
	}
	bind(&s, r)
	s, input := prepared(t, l, s, strings.Repeat("c", 32))
	f.writeReply = true
	r, observer, err := l.Execute(context.Background(), request(s, b.LifecycleStartTurn, b.LifecycleInput{Turn: &input}))
	if err != nil || observer == nil {
		t.Fatal("submission did not start observation", err)
	}
	defer observer.Close(context.Background())
	bind(&s, r)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	completed := 0
	for {
		select {
		case event, ok := <-observer.Events():
			if !ok {
				if completed != 1 {
					t.Fatal("native final was lost", completed)
				}
				return
			}
			if event.Fact.Kind == b.EventCompleted {
				completed++
				if !event.Fact.Completed || event.Fact.LastMessage != `{"summary":"original result"}` {
					t.Fatal("native final changed", event.Fact)
				}
			}
			event.Persisted <- nil
			event.Processed <- nil
		case err := <-observer.Done():
			if err != nil || completed != 1 {
				t.Fatal("native final delivery failed", err, completed)
			}
			return
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}
