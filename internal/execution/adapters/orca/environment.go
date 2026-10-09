package orcaworker

import (
	"context"
	"encoding/json"
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// InspectAgentInput relies on Orca's agent-independent TUI readiness gate.
func InspectAgentInput(ctx context.Context, call MetaCall, identity TerminalIdentity) (executionbackend.HostFact, bool, error) {
	fact, err := ObserveHost(ctx, call, identity)
	if err != nil || fact.Reason != "" {
		return fact, false, err
	}
	result, err := call(ctx, "terminal", "wait", "--terminal", identity.Handle, "--for", "tui-idle", "--timeout-ms", "1", "--json")
	if err != nil {
		return fact, false, err
	}
	var observed struct {
		Wait struct {
			Satisfied bool `json:"satisfied"`
		} `json:"wait"`
	}
	if json.Unmarshal(result.Payload, &observed) != nil || result.RuntimeID != identity.RuntimeID {
		return fact, false, errors.New("Orca input readiness changed its runtime identity")
	}
	if !observed.Wait.Satisfied {
		fact.Reason, fact.Confidence = "agent_input_unverified", "unverified"
	}
	return fact, observed.Wait.Satisfied, nil
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
