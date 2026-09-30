package codexworker

import (
	"encoding/json"
	"errors"
	"regexp"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
)

var nativeID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type AskEnvelope struct {
	RequestID json.RawMessage
	ThreadID  string
	TurnID    string
	CallID    string
	Arguments json.RawMessage
}

// ParseAskEnvelope verifies the native tool and turn binding. The caller
// validates the question content as an application Decision.
func ParseAskEnvelope(envelope codexapp.Envelope, expectedThread, expectedTurn string) (AskEnvelope, error) {
	if envelope.Method != "item/tool/call" || len(envelope.ID) == 0 || !json.Valid(envelope.ID) || !nativeID.MatchString(expectedThread) || !nativeID.MatchString(expectedTurn) {
		return AskEnvelope{}, errors.New("Codex ask event lacks a fixed native request or turn")
	}
	var params struct {
		ThreadID  string          `json:"threadId"`
		TurnID    string          `json:"turnId"`
		CallID    string          `json:"callId"`
		Namespace *string         `json:"namespace"`
		Tool      string          `json:"tool"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(envelope.Params, &params); err != nil {
		return AskEnvelope{}, err
	}
	if params.ThreadID != expectedThread || params.TurnID != expectedTurn || params.CallID == "" || params.Namespace != nil || params.Tool != codexapp.AskToolName {
		return AskEnvelope{}, errors.New("Codex ask request differs from the fixed tool, thread or turn")
	}
	return AskEnvelope{
		RequestID: append(json.RawMessage(nil), envelope.ID...),
		ThreadID:  params.ThreadID,
		TurnID:    params.TurnID,
		CallID:    params.CallID,
		Arguments: append(json.RawMessage(nil), params.Arguments...),
	}, nil
}
