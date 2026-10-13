package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"

	native "github.com/erranliu/SteerLine/internal/execution/adapters/protocol/orca"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

type integrationStateFeed struct {
	directory string
	initial   native.AgentState
	seen      int
	phase     string
	queryHook func()
}

func integrationPromptPreview(text string) string {
	units := utf16.Encode([]rune(strings.Join(strings.Fields(text), " ")))
	if len(units) > 200 {
		units = units[:200]
		if units[len(units)-1] >= 0xd800 && units[len(units)-1] <= 0xdbff {
			units = units[:199]
		}
	}
	return string(utf16.Decode(units))
}
func integrationAgentState(state terminalFixture, kind string) native.AgentState {
	prompt := ""
	if len(state.Prompts) > 0 {
		prompt = state.Prompts[len(state.Prompts)-1]
	}
	stamp := int64(state.Sends * 1000)
	rev := int64(state.Sends*10 + 1)
	if kind == "done" {
		stamp += 500
		rev++
	}
	s := native.AgentState{PaneKey: "tab:pane", AgentType: "codex", State: kind, Prompt: integrationPromptPreview(prompt), UpdatedAt: stamp, StateStartedAt: stamp, MainAgent: &native.MainAgentState{State: kind, StateStartedAt: stamp}, Observation: &native.StateObservation{Origin: "hook", Kind: "transition", AuthorityID: "fixture-hooks", Revision: rev, ObservedAt: stamp}}
	s.ProviderSession.ID = "fixture-session"
	if kind == "done" {
		s.LastAssistantMessage = state.NativeFinal
	}
	return s
}
func (f *integrationStateFeed) read() (terminalFixture, string, error) {
	var state terminalFixture
	raw, err := os.ReadFile(filepath.Join(f.directory, "transport.json"))
	if err != nil {
		return state, "", err
	}
	if err = json.Unmarshal(raw, &state); err != nil {
		return state, "", err
	}
	phase := "reply"
	if state.NativeGate && state.Sends == 3 {
		raw, err = os.ReadFile(filepath.Join(f.directory, "gate-phase"))
		if err != nil {
			return state, "", err
		}
		phase = strings.TrimSpace(string(raw))
	}
	return state, phase, nil
}
func (f *integrationStateFeed) Initial() native.AgentState { return f.initial }
func (f *integrationStateFeed) Drain() ([]native.AgentState, error) {
	state, phase, err := f.read()
	if err != nil {
		return nil, err
	}
	var updates []native.AgentState
	if state.Sends > f.seen {
		updates = append(updates, integrationAgentState(state, "working"))
		f.seen = state.Sends
		f.phase = ""
	}
	if state.Sends > 0 && phase != f.phase {
		hint := integrationAgentState(state, "done")
		hint.MainAgent = nil
		hint.Observation = nil
		hint.LastAssistantMessage = ""
		if phase == "working" {
			hint.StateStartedAt -= 400
			hint.UpdatedAt -= 400
		}
		updates = append(updates, hint)
		f.phase = phase
	}
	return updates, nil
}
func (f *integrationStateFeed) Query(context.Context) (native.AgentState, error) {
	if f.queryHook != nil {
		f.queryHook()
	}
	state, phase, err := f.read()
	if err != nil {
		return native.AgentState{}, err
	}
	kind := "done"
	if phase == "working" {
		kind = "working"
	}
	s := integrationAgentState(state, kind)
	s.Observation = nil
	return s, nil
}
func (*integrationStateFeed) Close() {}
func integrationStateFactory(directory string, queryHook func()) native.StateFeedFactory {
	return func(_ context.Context, _ string, _ b.ProcessIdentity) (native.StateFeed, error) {
		f := &integrationStateFeed{directory: directory, queryHook: queryHook}
		state, phase, err := f.read()
		if err != nil {
			return nil, err
		}
		kind := "done"
		if phase == "working" {
			kind = "working"
		}
		f.initial = integrationAgentState(state, kind)
		f.seen = state.Sends
		f.phase = phase
		return f, nil
	}
}
