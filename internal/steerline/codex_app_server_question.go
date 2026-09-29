package steerline

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const codexAskToolName = "steerline_ask"

type codexAskQuestion struct {
	ID            string           `json:"id"`
	Header        string           `json:"header"`
	Question      string           `json:"question"`
	Options       []DecisionOption `json:"options"`
	AllowFreeText bool             `json:"allow_free_text"`
}

type codexAskCall struct {
	RequestID  json.RawMessage    `json:"request_id"`
	ThreadID   string             `json:"thread_id"`
	TurnID     string             `json:"turn_id"`
	CallID     string             `json:"call_id"`
	Generation string             `json:"generation,omitempty"`
	Questions  []codexAskQuestion `json:"questions"`
}

// A client-provided dynamic tool is available in Codex Default mode. The
// built-in request_user_input tool is unavailable there on the probed release.
// The result is still a native app-server server request bound to a live turn.
func codexAskDynamicTool() any {
	option := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"id": map[string]any{"type": "string"}, "label": map[string]any{"type": "string"}, "detail": map[string]any{"type": "string"}}, "required": []string{"id", "label"}}
	question := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"id": map[string]any{"type": "string"}, "header": map[string]any{"type": "string"}, "question": map[string]any{"type": "string"}, "options": map[string]any{"type": "array", "items": option}, "allow_free_text": map[string]any{"type": "boolean"}}, "required": []string{"id", "header", "question", "options", "allow_free_text"}}
	return map[string]any{"type": "function", "name": codexAskToolName, "description": "Ask the user for a decision needed to continue the current task. Include every related question in one call, offer clear options when useful, and wait for the response. Use only when the user must decide.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"questions": map[string]any{"type": "array", "minItems": 1, "maxItems": 3, "items": question}}, "required": []string{"questions"}}}
}

func parseCodexAskCall(envelope codexAppEnvelope, expectedThread, expectedTurn string) (codexAskCall, error) {
	var call codexAskCall
	if envelope.Method != "item/tool/call" || len(envelope.ID) == 0 || !json.Valid(envelope.ID) || !codexThreadID.MatchString(expectedThread) || !codexThreadID.MatchString(expectedTurn) {
		return call, errors.New("Codex ask event lacks a fixed native request or turn")
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
		return call, err
	}
	if params.ThreadID != expectedThread || params.TurnID != expectedTurn || params.CallID == "" || params.Namespace != nil || params.Tool != codexAskToolName {
		return call, errors.New("Codex ask request differs from the fixed tool, thread or turn")
	}
	var args struct {
		Questions []codexAskQuestion `json:"questions"`
	}
	if err := decode(params.Arguments, &args); err != nil {
		return call, fmt.Errorf("Codex ask arguments: %w", err)
	}
	if len(args.Questions) == 0 || len(args.Questions) > 3 {
		return call, errors.New("Codex ask needs one to three questions")
	}
	seen := map[string]bool{}
	for i := range args.Questions {
		q := &args.Questions[i]
		q.ID = strings.TrimSpace(q.ID)
		q.Header = strings.TrimSpace(q.Header)
		q.Question = strings.TrimSpace(q.Question)
		if q.ID == "" || len(q.ID) > 128 || seen[q.ID] || q.Header == "" || len([]rune(q.Header)) > 40 || q.Question == "" || len([]rune(q.Question)) > 2000 {
			return call, fmt.Errorf("Codex ask question %d has invalid identity or text", i+1)
		}
		seen[q.ID] = true
		req := DecisionRequest{Question: q.Question, Options: q.Options, AllowFreeText: q.AllowFreeText}
		if err := validateDecisionRequest(&req); err != nil {
			return call, fmt.Errorf("Codex ask question %q: %w", q.ID, err)
		}
		q.Options = req.Options
	}
	call = codexAskCall{RequestID: append(json.RawMessage(nil), envelope.ID...), ThreadID: params.ThreadID, TurnID: params.TurnID, CallID: params.CallID, Questions: args.Questions}
	request := codexAskDecisionRequest(call)
	request.NativeAsk = nil // The process generation is attached by the service writer.
	if err := validateDecisionRequest(&request); err != nil {
		return codexAskCall{}, fmt.Errorf("Codex ask cannot be displayed completely: %w", err)
	}
	return call, nil
}

func codexAskDecisionRequest(call codexAskCall) DecisionRequest {
	request := DecisionRequest{Context: "Codex 正在等待回答；提交后会继续当前轮次。", AllowFreeText: true, NativeAsk: &call}
	if len(call.Questions) == 1 {
		q := call.Questions[0]
		request.Question = q.Question
		request.Options = append([]DecisionOption(nil), q.Options...)
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

func codexAskToolResponse(answers map[string][]string) map[string]any {
	return map[string]any{"contentItems": []map[string]string{{"type": "inputText", "text": string(mustJSON(map[string]any{"answers": answers}))}}, "success": true}
}
