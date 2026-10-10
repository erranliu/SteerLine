package orca_remote_worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
	m "github.com/erranliu/SteerLine/internal/execution/model"
)

const resultSource = "orca-supervised.transcript.step-reply"

// Persist native completion before acknowledging its delivery. The summary is
// not the business reply, which is collected separately from the transcript.
func captureResult(turn *orcaworker.NativeTurnState, message Message, process b.ProcessIdentity, outcome string) {
	wire, err := json.Marshal(message)
	if err != nil {
		turn.FinalReadError = "completion: worker_done could not be serialized"
		return
	}
	digest := sha256.Sum256(wire)
	fingerprint := hex.EncodeToString(digest[:])
	if turn.DoneMessageID != "" {
		if turn.DoneMessageID != message.ID || turn.ResultMessageDigest != fingerprint {
			turn.FinalReadError, turn.FinalVerified = "completion: conflicting original worker_done", false
		}
		return
	}
	turn.DoneMessageID, turn.ResultMessageDigest = message.ID, fingerprint
	turn.Outcome, turn.LastMessage = outcome, message.Body
	turn.FinalVerified = false
	if turn.ResultProcess == nil || *turn.ResultProcess != process || turn.TaskID != process.Task || turn.DispatchID != process.Dispatch ||
		message.RunID != process.OrchestrationRun || message.ToHandle != "run:"+process.OrchestrationRun ||
		!orcaworker.MessageMatches(message.FromHandle, message.Type, orcaworker.MessagePayload(message), turn.TaskID, turn.DispatchID, process.Terminal) {
		turn.FinalReadError = "completion: worker_done differs from its original Run/Task/Dispatch/worker binding"
		return
	}
	if len(wire) > MaxMessageBytes {
		turn.FinalReadError = "completion: worker_done serialized message exceeds 8 KiB"
		return
	}
	if !utf8.Valid(orcaworker.MessagePayload(message)) {
		turn.FinalReadError = "completion: worker_done payload is invalid UTF-8"
		return
	}
	_, err = decodeResultObject(orcaworker.MessagePayload(message))
	if err != nil {
		turn.FinalReadError = "completion: invalid worker_done payload: " + err.Error()
		return
	}
	turn.FinalReadError = ""
}

// Native completion payloads must have unambiguous keys.
func decodeResultObject(raw []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, errors.New("worker result must be an object")
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if _, seen := fields[key]; !ok || seen {
			return nil, errors.New("worker result has duplicate fields")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		fields[key] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		return nil, errors.New("worker result has trailing JSON")
	}
	return fields, nil
}

func settledResult(snapshot b.LifecycleSnapshot, turn *orcaworker.NativeTurnState) (b.TurnEvidence, error) {
	if turn.ResultProcess == nil || *turn.ResultProcess != snapshot.Ref.External || !b.StableExecutionID(turn.TurnID) || turn.InputVersion == 0 || turn.TaskID != snapshot.Ref.External.Task || turn.DispatchID != snapshot.Ref.External.Dispatch || turn.DoneMessageID == "" || !turn.Settled {
		return b.TurnEvidence{}, errors.New("settled result lost its original worker/Command binding")
	}
	if strings.HasPrefix(turn.FinalReadError, "completion:") {
		return b.TurnEvidence{}, errors.New(turn.FinalReadError)
	}
	valid := turn.FinalReadError == "" && turn.FinalSource == resultSource && turn.FinalItemID != "" &&
		turn.FinalNativeTurnID == turn.DispatchID && turn.FinalText != "" && turn.ResultMessageDigest != "" && turn.InputAnchorID != "" && turn.OutputSourceIdentity != ""
	turn.FinalVerified = valid
	if !valid && turn.FinalReadError == "" {
		turn.FinalReadError = "original step reply is not available in its bound transcript"
	}
	return b.TurnEvidence{FinalOrigin: "worker_declared", FinalDiagnostic: turn.FinalReadError,
		FinalSource: turn.FinalSource, FinalItemID: turn.FinalItemID, FinalVerified: valid,
		FinalNativeSessionID: SessionID(*turn.ResultProcess), FinalNativeTurnID: turn.FinalNativeTurnID,
		TurnID: turn.TurnID, CommandID: turn.TurnID, SessionID: SessionID(*turn.ResultProcess),
		Started: true, Completed: true, Succeeded: turn.Outcome == "succeeded", LastMessage: turn.FinalText,
		PRCandidates: b.PRClues(turn.FinalText)}, nil
}

type transcriptMessage struct {
	ID     string `json:"id"`
	Role   string `json:"role"`
	Blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"blocks"`
}

// The native Task preamble anchors each input in a reused session. Only text
// replies in that segment are collected; tool output and completion summaries
// never supply the business result. No provider-specific history is read.
func (c Client) captureReply(ctx context.Context, turn *orcaworker.NativeTurnState) {
	if turn.FinalVerified {
		return
	}
	body, item, anchor, source, err := c.readStepReply(ctx, turn)
	if anchor != "" {
		turn.InputAnchorID = anchor
	}
	if source != "" {
		turn.OutputSourceIdentity = source
	}
	if err != nil {
		turn.FinalReadError = err.Error()
		return
	}
	turn.FinalText, turn.FinalItemID = body, item
	turn.InputAnchorID, turn.OutputSourceIdentity = anchor, source
	turn.FinalSource, turn.FinalNativeTurnID, turn.FinalReadError = resultSource, turn.DispatchID, ""
}

func (c Client) readStepReply(ctx context.Context, turn *orcaworker.NativeTurnState) (body, item, anchor, source string, err error) {
	cursor := ""
	seen := map[string]string{}
	found := false
	clipped := false
	for page := 0; page < 1000; page++ {
		args := []string{"orchestration", "worker-read", "--dispatch", turn.DispatchID, "--source", "transcript", "--limit", "100", "--json"}
		if cursor != "" {
			args = append(args, "--cursor", cursor)
		}
		env, readErr := c.CenterCall(ctx, args...)
		if readErr != nil {
			return "", "", anchor, source, readErr
		}
		var output struct {
			DispatchID     string `json:"dispatchId"`
			Source         string `json:"source"`
			SourceIdentity string `json:"sourceIdentity"`
			SourceExact    bool   `json:"sourceExact"`
			RemoteRuntime  string `json:"remoteRuntimeEpoch"`
			Transcript     struct {
				Messages   []transcriptMessage `json:"messages"`
				NextCursor string              `json:"nextCursor"`
			} `json:"transcript"`
		}
		if err := json.Unmarshal(env.Payload, &output); err != nil {
			return "", "", anchor, source, err
		}
		if turn.ResultProcess == nil || env.RuntimeID != turn.ResultProcess.CoordinatorRuntimeID ||
			output.DispatchID != turn.DispatchID || output.Source != "transcript" || !output.SourceExact || output.SourceIdentity == "" ||
			!c.config.local() && output.RemoteRuntime != turn.ResultProcess.RuntimeID ||
			c.config.local() && env.RuntimeID != turn.ResultProcess.RuntimeID ||
			source != "" && source != output.SourceIdentity || turn.OutputSourceIdentity != "" && turn.OutputSourceIdentity != output.SourceIdentity {
			return "", "", anchor, source, errors.New("step reply transcript changed its original source/runtime/Dispatch")
		}
		source = output.SourceIdentity
		for _, message := range output.Transcript.Messages {
			wire, _ := json.Marshal(message)
			if message.ID == "" {
				return "", "", anchor, source, errors.New("step reply transcript omitted message identity")
			}
			if previous, ok := seen[message.ID]; ok {
				if previous != string(wire) {
					return "", "", anchor, source, errors.New("conflicting transcript message")
				}
				continue
			}
			seen[message.ID] = string(wire)
			text, textOnly, textClipped := "", len(message.Blocks) > 0, false
			for _, block := range message.Blocks {
				textOnly = textOnly && block.Type == "text"
				if block.Type == "text" {
					text += block.Text
					textClipped = textClipped || strings.HasSuffix(strings.TrimSpace(block.Text), "… (truncated)")
				}
			}
			if message.Role == "user" && strings.HasPrefix(text, "You are working inside Orca, a multi-agent IDE. You are a dispatched worker.") {
				matches := false
				for _, line := range strings.Split(text, "\n") {
					matches = matches || strings.TrimSpace(line) == "Your task ID is: "+turn.TaskID
				}
				if matches {
					if found || turn.InputAnchorID != "" && turn.InputAnchorID != message.ID {
						return "", "", anchor, source, errors.New("step reply has conflicting original input anchors")
					}
					found, anchor, body, item = true, message.ID, "", ""
				} else if found {
					return "", "", anchor, source, errors.New("original step reply was not frozen before another native Task")
				}
				continue
			}
			if !found {
				continue
			}
			if message.Role == "user" {
				return "", "", anchor, source, errors.New("step reply crossed an unbound user input")
			}
			if message.Role == "tool" || message.Role == "assistant" && !textOnly {
				body, item = "", "" // Wait for the reply after the last tool call.
				clipped = false
			} else if message.Role == "assistant" && strings.TrimSpace(text) != "" {
				body, item = text, message.ID
				clipped = textClipped
			}
		}
		next := output.Transcript.NextCursor
		if next == "" || next == cursor {
			if !found || body == "" || !utf8.ValidString(body) || len(body) > m.MaxConversationBodyBytes || clipped {
				return "", "", anchor, source, errors.New("original step reply is missing, clipped or exceeds the body budget")
			}
			return body, item, anchor, source, nil
		}
		cursor = next
	}
	return "", "", anchor, source, errors.New("step reply transcript pagination did not finish")
}
