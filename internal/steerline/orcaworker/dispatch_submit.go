package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Orca's inject receipt can stop at input_accepted while Codex still has the
// prompt in its composer. Only a visible, unsubmitted paste permits one Enter.
func PromptAwaitingSubmission(receipt json.RawMessage, incarnation string) bool {
	var result struct {
		Prompt struct {
			Provider           string   `json:"provider"`
			ProcessIncarnation string   `json:"processIncarnation"`
			Stages             []string `json:"stages"`
		} `json:"prompt"`
	}
	if json.Unmarshal(receipt, &result) != nil || result.Prompt.Provider != "codex" ||
		result.Prompt.ProcessIncarnation != incarnation {
		return false
	}
	accepted, started := false, false
	for _, stage := range result.Prompt.Stages {
		accepted = accepted || stage == "input_accepted"
		started = started || stage == "turn_started"
	}
	return accepted && !started
}

func screenHasPendingPaste(lines []string) bool {
	for _, line := range lines {
		count, ok := strings.CutPrefix(strings.TrimSpace(line), "› [Pasted Content ")
		if !ok || !strings.HasSuffix(count, " chars]") {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimSuffix(count, " chars]")); err == nil && n > 0 {
			return true
		}
	}
	return false
}

func SubmitPendingPaste(ctx context.Context, call MetaCall, identity TerminalIdentity) (bool, error) {
	if identity.Kind != "orca-native" || identity.Handle == "" || identity.RuntimeID == "" || identity.Incarnation == "" {
		return false, errors.New("native Orca prompt has no fixed worker terminal")
	}
	for attempt := 0; attempt < 4; attempt++ {
		env, err := call(ctx, "terminal", "read", "--terminal", identity.Handle, "--json")
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
		if json.Unmarshal(env.Payload, &result) != nil || env.RuntimeID != identity.RuntimeID ||
			result.Terminal.Handle != identity.Handle {
			return false, &PlatformError{Code: "identity_mismatch", Diagnostic: "Orca prompt screen came from a different worker terminal"}
		}
		if result.Terminal.Source == "screen" && screenHasPendingPaste(result.Terminal.Tail) {
			term, err := ShowTerminal(ctx, call, identity.Handle)
			if err != nil {
				return false, err
			}
			if term.RuntimeID != identity.RuntimeID || term.IncarnationID != identity.Incarnation {
				return false, &PlatformError{Code: "identity_mismatch", Diagnostic: "Orca pending prompt terminal was replaced"}
			}
			sent, err := call(ctx, "terminal", "send", "--terminal", identity.Handle, "--enter", "--json")
			if err != nil {
				return false, err
			}
			var reply struct {
				Send struct {
					Handle   string `json:"handle"`
					Accepted bool   `json:"accepted"`
				} `json:"send"`
			}
			if json.Unmarshal(sent.Payload, &reply) != nil || sent.RuntimeID != identity.RuntimeID ||
				reply.Send.Handle != identity.Handle || !reply.Send.Accepted {
				return false, &PlatformError{Code: "outcome_unknown", Diagnostic: "Orca did not confirm submission of the pasted worker prompt"}
			}
			return true, nil
		}
		if attempt < 3 {
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-time.After(250 * time.Millisecond):
			}
		}
	}
	return false, nil
}
