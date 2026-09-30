package codexworker

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
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

func ParseAskCall(envelope codexapp.Envelope, threadID, turnID string) (contracts.CodexAskCall, error) {
	wire, err := ParseAskEnvelope(envelope, threadID, turnID)
	if err != nil {
		return contracts.CodexAskCall{}, err
	}
	return AskCall(wire)
}
func AskCall(wire AskEnvelope) (contracts.CodexAskCall, error) {
	questions, err := DecodeQuestions(wire.Arguments)
	if err != nil {
		return contracts.CodexAskCall{}, err
	}
	if len(questions) == 0 || len(questions) > 3 {
		return contracts.CodexAskCall{}, errors.New("Codex ask needs one to three questions")
	}
	seen := map[string]bool{}
	for i := range questions {
		q := &questions[i]
		q.ID = strings.TrimSpace(q.ID)
		q.Header = strings.TrimSpace(q.Header)
		q.Question = strings.TrimSpace(q.Question)
		if q.ID == "" || len(q.ID) > 128 || seen[q.ID] || q.Header == "" || len([]rune(q.Header)) > 40 || q.Question == "" || len([]rune(q.Question)) > 2000 {
			return contracts.CodexAskCall{}, fmt.Errorf("Codex ask question %d has invalid identity or text", i+1)
		}
		seen[q.ID] = true
		req := contracts.DecisionRequest{Question: q.Question, Options: q.Options, AllowFreeText: q.AllowFreeText}
		if err := contracts.NormalizeDecisionContent(&req); err != nil {
			return contracts.CodexAskCall{}, fmt.Errorf("Codex ask question %q: %w", q.ID, err)
		}
		q.Options = req.Options
	}
	call := contracts.CodexAskCall{RequestID: wire.RequestID, ThreadID: wire.ThreadID, TurnID: wire.TurnID, CallID: wire.CallID, Questions: questions}
	request := AskDecisionRequest(call)
	request.NativeAsk = nil
	if err := contracts.NormalizeDecisionContent(&request); err != nil {
		return contracts.CodexAskCall{}, fmt.Errorf("Codex ask cannot be displayed completely: %w", err)
	}
	return call, nil
}
func AskDecisionRequest(call contracts.CodexAskCall) contracts.DecisionRequest {
	request := contracts.DecisionRequest{Context: "Codex 正在等待回答；提交后会继续当前轮次。", AllowFreeText: true, NativeAsk: &call}
	if len(call.Questions) == 1 {
		q := call.Questions[0]
		request.Question = q.Question
		request.Options = append([]executionapi.DecisionOption(nil), q.Options...)
		request.AllowFreeText = q.AllowFreeText
		return request
	}
	request.Question = fmt.Sprintf("Codex 有 %d 个问题需要回答", len(call.Questions))
	var body strings.Builder
	body.WriteString(request.Context)
	body.WriteString(" 多题请在备注中提交 JSON，例如 {\"question_id\":[\"answer\"]}，每个问题 ID 都须填写。")
	for _, q := range call.Questions {
		fmt.Fprintf(&body, "\n\n[%s] %s：%s", q.ID, q.Header, q.Question)
		for _, option := range q.Options {
			fmt.Fprintf(&body, "\n- %s：%s", option.ID, option.Label)
			if option.Detail != "" {
				fmt.Fprintf(&body, " — %s", option.Detail)
			}
		}
		if q.AllowFreeText {
			body.WriteString("\n允许自由文字回答。")
		} else {
			body.WriteString("\n请选择以上选项，可附加备注。")
		}
	}
	request.Context = body.String()
	return request
}

// ValidateSavedAsk verifies only native binding; the center separately owns
// decision authorization, accepted answers and Execution/Attempt eligibility.
func ValidateSavedAsk(ask contracts.CodexAskCall) error {
	if !ValidNativeID(ask.ThreadID) || !ValidNativeID(ask.TurnID) || ask.CallID == "" || ask.Generation == "" || len(ask.RequestID) == 0 || !json.Valid(ask.RequestID) || len(ask.Questions) == 0 || len(ask.Questions) > 3 {
		return errors.New("native decision lacks its fixed app-server request binding")
	}
	return nil
}
