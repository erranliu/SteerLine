package orca_remote_worker

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

const resultSource = "orca-supervised.worker-declared"

type declaredResult struct {
	Version      int    `json:"version"`
	CommandID    string `json:"command_id"`
	InputVersion uint64 `json:"input_version"`
	Body         string `json:"body"`
}

func resultPrompt(input b.TurnInput) string {
	return input.Prompt + fmt.Sprintf(`
SteerLine result contract: before ending this turn, send the native worker_done exactly once with its normal three-sentence summary in --body. Use one complete --payload JSON object containing the original taskId, dispatchId, outcome, and steerline_result={"version":1,"command_id":%q,"input_version":%d,"body":"<complete response>"}. Copy all native sender/authority arguments from the Orca preamble. Do NOT combine --payload with --task-id, --dispatch-id, --outcome, --phase, --files-modified or --report-path; put any required payload fields inside that object. Construct JSON with a proper serializer and pass it as one native argument, preserving quotes and Unicode. Keep the full serialized message including summary/payload within 8192 UTF-8 bytes; do not truncate or split it. The result body is your complete normal reply for implement.execute, and the exact complete structured report requested for report/publish. A completion summary is not the result body. If reporting cannot be completed, keep the failure visible and do not claim success.
`, input.CommandID, input.InputVersion)
}

// Freeze the explicit body before the Delivery can be acknowledged. Completion
// is separately proven by the original Dispatch's settlement, never this text.
func captureResult(turn *orcaworker.NativeTurnState, message Message, process b.ProcessIdentity, outcome string) {
	wire, err := json.Marshal(message)
	if err != nil {
		turn.FinalReadError = "worker_done could not be serialized"
		return
	}
	digest := sha256.Sum256(wire)
	fingerprint := hex.EncodeToString(digest[:])
	if turn.DoneMessageID != "" {
		if turn.DoneMessageID != message.ID || turn.ResultMessageDigest != fingerprint {
			turn.FinalReadError, turn.FinalVerified = "conflicting original worker_done result", false
		}
		return
	}
	turn.DoneMessageID, turn.ResultMessageDigest = message.ID, fingerprint
	turn.Outcome, turn.LastMessage = outcome, message.Body
	turn.FinalVerified = false
	if turn.ResultProcess == nil || *turn.ResultProcess != process || turn.TaskID != process.Task || turn.DispatchID != process.Dispatch ||
		message.RunID != process.OrchestrationRun || message.ToHandle != "run:"+process.OrchestrationRun ||
		!orcaworker.MessageMatches(message.FromHandle, message.Type, orcaworker.MessagePayload(message), turn.TaskID, turn.DispatchID, process.Terminal) {
		turn.FinalReadError = "worker_done differs from its original Run/Task/Dispatch/worker binding"
		return
	}
	if len(wire) > MaxMessageBytes {
		turn.FinalReadError = "worker_done serialized message exceeds 8 KiB"
		return
	}
	if !utf8.Valid(orcaworker.MessagePayload(message)) {
		turn.FinalReadError = "worker result payload is invalid UTF-8"
		return
	}
	payload, err := decodeResultObject(orcaworker.MessagePayload(message))
	if err != nil {
		turn.FinalReadError = "invalid worker_done payload: " + err.Error()
		return
	}
	if len(payload["steerline_result"]) == 0 {
		turn.FinalReadError = "original worker_done has no complete steerline_result"
		return
	}
	var result declaredResult
	if err := decodeDeclaredResult(payload["steerline_result"], &result); err != nil {
		turn.FinalReadError = "invalid steerline_result: " + err.Error()
		return
	}
	if result.Version != 1 || !b.StableExecutionID(result.CommandID) || result.CommandID != turn.TurnID || result.InputVersion == 0 || result.InputVersion != turn.InputVersion {
		turn.FinalReadError = "worker result differs from the original Command/input version"
		return
	}
	if strings.TrimSpace(result.Body) == "" || !utf8.ValidString(result.Body) {
		turn.FinalReadError = "worker result body is empty or invalid UTF-8"
		return
	}
	turn.FinalText, turn.FinalSource, turn.FinalItemID = result.Body, resultSource, message.ID
	turn.FinalNativeTurnID, turn.FinalReadError = process.Dispatch, ""
}

func decodeDeclaredResult(raw []byte, result *declaredResult) error {
	fields, err := decodeResultObject(raw)
	if err != nil {
		return err
	}
	for key := range fields {
		switch key {
		case "version", "command_id", "input_version", "body":
		default:
			return errors.New("worker result has unknown fields")
		}
	}
	if len(fields) != 4 {
		return errors.New("worker result lacks required fields")
	}
	return b.DecodeStrictJSON(raw, result, "worker result has trailing JSON")
}

// Both the native payload and its result must have unambiguous, exact keys.
// Native payloads may carry additional Orca fields outside steerline_result.
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
	if turn.ResultProcess == nil || *turn.ResultProcess != snapshot.Ref.External || turn.TurnID == "" || turn.DoneMessageID == "" || !turn.Settled {
		return b.TurnEvidence{}, errors.New("settled result lost its original worker/Command binding")
	}
	valid := turn.FinalReadError == "" && turn.FinalSource == resultSource && turn.FinalItemID == turn.DoneMessageID &&
		turn.FinalNativeTurnID == turn.DispatchID && turn.FinalText != "" && turn.ResultMessageDigest != ""
	turn.FinalVerified = valid
	if !valid && turn.FinalReadError == "" {
		turn.FinalReadError = "original worker_done lacks bound complete result evidence"
	}
	return b.TurnEvidence{FinalOrigin: "worker_declared", FinalDiagnostic: turn.FinalReadError,
		FinalSource: turn.FinalSource, FinalItemID: turn.FinalItemID, FinalVerified: valid,
		FinalNativeSessionID: SessionID(*turn.ResultProcess), FinalNativeTurnID: turn.FinalNativeTurnID,
		TurnID: turn.TurnID, CommandID: turn.TurnID, SessionID: SessionID(*turn.ResultProcess),
		Started: true, Completed: true, Succeeded: turn.Outcome == "succeeded", LastMessage: turn.FinalText,
		PRCandidates: b.PRClues(turn.FinalText)}, nil
}
