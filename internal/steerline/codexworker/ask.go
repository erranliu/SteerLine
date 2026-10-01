package codexworker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

const QuestionRouteImplementationKey = "codex.appserver"

type AskRouteData struct {
	RequestID  json.RawMessage `json:"request_id"`
	ThreadID   string          `json:"thread_id"`
	TurnID     string          `json:"turn_id"`
	CallID     string          `json:"call_id"`
	Generation string          `json:"-"`
}

type AskCall struct {
	Route     AskRouteData
	Questions []executionapi.Question
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

func ParseAskCall(envelope codexapp.Envelope, threadID, turnID string) (AskCall, error) {
	wire, err := ParseAskEnvelope(envelope, threadID, turnID)
	if err != nil {
		return AskCall{}, err
	}
	return decodeAskCall(wire)
}
func decodeAskCall(wire AskEnvelope) (AskCall, error) {
	questions, err := DecodeQuestions(wire.Arguments)
	if err != nil {
		return AskCall{}, err
	}
	if len(questions) == 0 || len(questions) > 3 {
		return AskCall{}, errors.New("Codex ask needs one to three questions")
	}
	seen := map[string]bool{}
	for i := range questions {
		q := &questions[i]
		q.ID = strings.TrimSpace(q.ID)
		q.Header = strings.TrimSpace(q.Header)
		q.Question = strings.TrimSpace(q.Question)
		if q.ID == "" || len(q.ID) > 128 || seen[q.ID] || q.Header == "" || len([]rune(q.Header)) > 40 || q.Question == "" || len([]rune(q.Question)) > 2000 {
			return AskCall{}, fmt.Errorf("Codex ask question %d has invalid identity or text", i+1)
		}
		seen[q.ID] = true
		req := contracts.DecisionRequest{Question: q.Question, Options: q.Options, AllowFreeText: q.AllowFreeText}
		if err := contracts.NormalizeDecisionContent(&req); err != nil {
			return AskCall{}, fmt.Errorf("Codex ask question %q: %w", q.ID, err)
		}
		q.Options = req.Options
	}
	call := AskCall{Route: AskRouteData{RequestID: append(json.RawMessage(nil), wire.RequestID...), ThreadID: wire.ThreadID, TurnID: wire.TurnID, CallID: wire.CallID}, Questions: questions}
	request := decisionPresentation(questions)
	if err := contracts.NormalizeDecisionContent(&request); err != nil {
		return AskCall{}, fmt.Errorf("Codex ask cannot be displayed completely: %w", err)
	}
	return call, nil
}

func decisionPresentation(questions []executionapi.Question) contracts.DecisionRequest {
	request := contracts.DecisionRequest{FormatVersion: 2, Context: "执行当前问题需要人工回答；提交后将继续原执行。", AllowFreeText: true}
	if len(questions) == 1 {
		q := questions[0]
		request.Question = q.Question
		request.Options = append([]executionapi.DecisionOption(nil), q.Options...)
		request.AllowFreeText = q.AllowFreeText
		return request
	}
	request.Question = fmt.Sprintf("当前执行有 %d 个问题需要回答", len(questions))
	var body strings.Builder
	body.WriteString(request.Context)
	body.WriteString(" 多题请提交以问题 ID 为键的 JSON，并回答每个问题。")
	for _, q := range questions {
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

func NewQuestionRoute(call AskCall, fixed executionapi.SessionContext) (executionapi.QuestionRoute, error) {
	fixedTurn := fixed.CurrentTurnID
	if fixedTurn == "" {
		fixedTurn = fixed.PendingTurnID
	}
	if !ValidNativeID(call.Route.ThreadID) || !ValidNativeID(call.Route.TurnID) || call.Route.CallID == "" ||
		!json.Valid(call.Route.RequestID) || call.Route.ThreadID != fixed.SessionID || call.Route.TurnID != fixedTurn {
		return executionapi.QuestionRoute{}, errors.New("native question route differs from its fixed session or turn")
	}
	payload, err := json.Marshal(call.Route)
	if err != nil {
		return executionapi.QuestionRoute{}, err
	}
	return executionapi.NewQuestionRoute(QuestionRouteImplementationKey, 1, executionapi.QuestionRouteIdentity{
		ProjectID: fixed.ProjectID, WorkflowRunID: fixed.WorkflowRunID, ExecutionID: fixed.ExecutionID,
		AttemptID: fixed.AttemptID, Generation: fixed.Generation, TurnID: call.Route.TurnID, QuestionID: call.Route.CallID,
	}, payload)
}

func DecodeQuestionRoute(route executionapi.QuestionRoute, fixed executionapi.SessionContext) (AskRouteData, error) {
	want := executionapi.QuestionRouteIdentity{ProjectID: fixed.ProjectID, WorkflowRunID: fixed.WorkflowRunID,
		ExecutionID: fixed.ExecutionID, AttemptID: fixed.AttemptID, Generation: fixed.Generation,
		TurnID: route.TurnID, QuestionID: route.QuestionID}
	if route.ImplementationKey != QuestionRouteImplementationKey || route.CodecVersion != 1 || route.Purpose != executionapi.QuestionRouteAnswerPurpose {
		return AskRouteData{}, errors.New("unsupported native question route codec")
	}
	if err := route.ValidateIdentity(want); err != nil {
		return AskRouteData{}, err
	}
	var data AskRouteData
	decoder := json.NewDecoder(bytes.NewReader(route.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&data); err != nil {
		return AskRouteData{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return AskRouteData{}, errors.New("native question route has trailing data")
	}
	if !ValidNativeID(data.ThreadID) || !ValidNativeID(data.TurnID) || data.ThreadID != fixed.SessionID ||
		data.TurnID != route.TurnID || data.CallID != route.QuestionID || len(data.RequestID) == 0 || !json.Valid(data.RequestID) {
		return AskRouteData{}, errors.New("native question route payload changed its original request identity")
	}
	data.RequestID = append(json.RawMessage(nil), data.RequestID...)
	data.Generation = route.Generation
	return data, nil
}
