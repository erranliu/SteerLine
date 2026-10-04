package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// InspectAgentInput qualifies only a new input, not readiness or settlement.
// A stale agentIdentity can outlive Codex and refer to its parent shell. Keep
// source limitations explicit rather than injecting into a startup trust menu.
func InspectAgentInput(ctx context.Context, call MetaCall, identity TerminalIdentity) (executionbackend.HostFact, bool, error) {
	fact, err := ObserveHost(ctx, call, identity)
	if err != nil || fact.Reason != "" {
		return fact, false, err
	}
	envelope, err := call(ctx, "terminal", "read", "--terminal", identity.Handle, "--screen", "--json")
	if err != nil {
		return fact, false, err
	}
	var result struct {
		Terminal struct {
			Handle string   `json:"handle"`
			Source string   `json:"source"`
			Tail   []string `json:"tail"`
		} `json:"terminal"`
	}
	if json.Unmarshal(envelope.Payload, &result) != nil || envelope.RuntimeID != identity.RuntimeID || result.Terminal.Handle != identity.Handle {
		return fact, false, errors.New("agent input observation changed original identity")
	}
	fact.Source = "orca.terminal.read.input-screen"
	if result.Terminal.Source == "screen" {
		text := strings.ToLower(strings.Join(result.Terminal.Tail, "\n"))
		if strings.Contains(text, "trust and continue") && strings.Contains(text, "quit") {
			fact.Reason, fact.Confidence, fact.Detail = "host_wait", "suspected", "原 Codex 显示目录信任菜单；请在原生界面处理。"
			return fact, false, nil
		}
		busy := strings.Contains(text, "esc to interrupt") || strings.Contains(text, "working (") || strings.Contains(text, "running hook")
		codexChrome := strings.Contains(text, "? for shortcuts") || strings.Contains(text, "gpt-")
		for _, line := range result.Terminal.Tail {
			if !busy && codexChrome && strings.HasPrefix(strings.TrimSpace(line), "› ") {
				return fact, true, nil
			}
		}
	}
	fact.Reason, fact.Confidence, fact.Detail = "agent_input_unverified", "unverified", "原 terminal 的当前 Codex 输入入口尚未核实；不会向 shell 或启动菜单注入。"
	return fact, false, nil
}

// ObserveTurnRunning reports only explicit busy markers in the original Codex screen.
// A Dispatch acceptance or an idle prompt alone is not a turn-start receipt.
func ObserveTurnRunning(ctx context.Context, call MetaCall, identity TerminalIdentity) (bool, error) {
	fact, err := ObserveHost(ctx, call, identity)
	if err != nil {
		return false, err
	}
	if fact.Reason != "" {
		return false, &PlatformError{Code: "identity_mismatch", Diagnostic: fact.Reason}
	}
	envelope, err := call(ctx, "terminal", "read", "--terminal", identity.Handle, "--screen", "--json")
	if err != nil {
		return false, err
	}
	var result struct {
		Terminal struct {
			Handle string   `json:"handle"`
			Source string   `json:"source"`
			Tail   []string `json:"tail"`
		} `json:"terminal"`
	}
	if json.Unmarshal(envelope.Payload, &result) != nil || envelope.RuntimeID != identity.RuntimeID || result.Terminal.Handle != identity.Handle {
		return false, errors.New("turn-start observation changed original terminal identity")
	}
	if result.Terminal.Source != "screen" {
		return false, nil
	}
	text := strings.ToLower(strings.Join(result.Terminal.Tail, "\n"))
	return strings.Contains(text, "esc to interrupt") || strings.Contains(text, "working (") || strings.Contains(text, "running hook"), nil
}

// ObserveHost keeps exit and waiting facts even when sending is impossible.
// Unknown/disconnected observations are gaps, never a fabricated process exit.
func ObserveHost(ctx context.Context, call MetaCall, identity TerminalIdentity) (executionbackend.HostFact, error) {
	if identity.Handle == "" || identity.RuntimeID == "" || identity.Incarnation == "" {
		return executionbackend.HostFact{}, errors.New("host observation has no original terminal identity")
	}
	fact := executionbackend.HostFact{Runtime: identity.RuntimeID, Terminal: identity.Handle, Incarnation: identity.Incarnation, Source: "orca.terminal.show"}
	envelope, err := call(ctx, "terminal", "show", "--terminal", identity.Handle, "--json")
	if err != nil {
		return fact, err
	}
	var result struct {
		Terminal Terminal `json:"terminal"`
	}
	if err = json.Unmarshal(envelope.Payload, &result); err != nil {
		return fact, err
	}
	t := result.Terminal
	if envelope.RuntimeID != identity.RuntimeID || t.Handle != identity.Handle || t.IncarnationID != identity.Incarnation {
		fact.Reason, fact.Confidence = "identity_mismatch", "confirmed"
		return fact, nil
	}
	if t.Exited() {
		fact.Reason, fact.Confidence = "terminal_exited", "confirmed"
		return fact, nil
	}
	if t.Orphaned || !t.Connected {
		return fact, errors.New("original terminal observation is disconnected")
	}
	if t.AgentWait != nil && string(*t.AgentWait) != "null" && len(*t.AgentWait) <= 4096 {
		var wait struct {
			Kind   string `json:"kind"`
			Source string `json:"source"`
		}
		if json.Unmarshal(*t.AgentWait, &wait) != nil {
			return fact, errors.New("malformed terminal agentWait")
		}
		if wait.Kind != "" || wait.Source != "" {
			fact.Reason, fact.Confidence, fact.Detail = "host_wait", "suspected", wait.Kind
			fact.Source = "orca.terminal.show.agentWait:" + wait.Source
			// Only explicitly structured platform permission facts are confirmed.
			if wait.Source == "structured" && (wait.Kind == "host_permission" || wait.Kind == "approval") {
				fact.Confidence = "confirmed"
			}
		}
	}
	return fact, nil
}
