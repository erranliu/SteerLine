package orcatransport

import (
	"context"

	native "github.com/erranliu/SteerLine/internal/execution/adapters/protocol/orca"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

type hostStateFeed struct {
	host    *fakeHost
	initial native.AgentState
	seen    int
	hinted  bool
}

func fixtureState(seq int, prompt, state string) native.AgentState {
	s := native.AgentState{PaneKey: "tab:pane", AgentType: "codex", Prompt: preview(prompt), State: state, UpdatedAt: int64(seq * 1000), StateStartedAt: int64(seq * 1000), MainAgent: &native.MainAgentState{State: state, StateStartedAt: int64(seq * 1000)}, Observation: &native.StateObservation{Origin: "hook", Kind: "transition", AuthorityID: "hooks", Revision: int64(seq), ObservedAt: int64(seq * 1000)}}
	s.ProviderSession.ID = "provider"
	return s
}
func (f *fakeHost) stateFactory(_ context.Context, _ string, _ b.ProcessIdentity) (native.StateFeed, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := fixtureState(f.nativeSends*2, "", "done")
	return &hostStateFeed{host: f, initial: s}, nil
}
func (f *hostStateFeed) Initial() native.AgentState { return f.initial }
func (f *hostStateFeed) Drain() ([]native.AgentState, error) {
	f.host.mu.Lock()
	defer f.host.mu.Unlock()
	h := f.host
	var states []native.AgentState
	if h.nativeSends > f.seen {
		states = append(states, fixtureState(h.nativeSends*2-1, h.nativePrompt, "working"))
		f.seen = h.nativeSends
		f.hinted = false
	}
	if h.nativeSends > 0 && h.writeReply && !h.busy && !f.hinted {
		s := fixtureState(h.nativeSends*2, h.nativePrompt, "done")
		s.Observation = nil
		s.MainAgent = nil
		states = append(states, s)
		f.hinted = true
	}
	return states, nil
}
func (f *hostStateFeed) Query(context.Context) (native.AgentState, error) {
	f.host.mu.Lock()
	defer f.host.mu.Unlock()
	h := f.host
	state := "working"
	seq := h.nativeSends*2 - 1
	if h.writeReply && !h.busy {
		state = "done"
		seq++
	}
	s := fixtureState(seq, h.nativePrompt, state)
	if state == "done" {
		s.MainAgent.Outcome = "success"
		text, _, _ := renderedReply("codex", h.lines)
		s.LastAssistantMessage = text
	}
	return s, nil
}
func (*hostStateFeed) Close() {}
