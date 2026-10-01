package codexworker

import (
	"encoding/json"
	"errors"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

type EventFact struct {
	executionapi.EventFact
}

// DecodeEvent checks native identities before exposing facts to the center.
func DecodeEvent(envelope codexapp.Envelope, fixed executionapi.SessionContext, inspectReply bool) (EventFact, error) {
	var fact EventFact
	threadID := fixed.SessionID
	turnID := fixed.CurrentTurnID
	if turnID == "" {
		turnID = fixed.PendingTurnID
	}
	fact.SessionID = threadID
	switch envelope.Method {
	case "turn/started":
		var params struct {
			ThreadID string `json:"threadId"`
			Turn     struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		if json.Unmarshal(envelope.Params, &params) != nil || params.ThreadID != threadID || !nativeID.MatchString(params.Turn.ID) {
			return fact, errors.New("app-server turn start changed its fixed thread")
		}
		fact.StartedTurnID = params.Turn.ID
		fact.Kind = executionapi.EventStarted
	case "item/tool/call":
		call, err := ParseAskCall(envelope, threadID, turnID)
		if err != nil {
			return fact, err
		}
		route, err := NewQuestionRoute(call, fixed)
		if err != nil {
			return fact, err
		}
		for i := range call.Questions {
			call.Questions[i].ExecutionID = fixed.ExecutionID
			call.Questions[i].AttemptID = fixed.AttemptID
			call.Questions[i].Generation = fixed.Generation
			call.Questions[i].TurnID = call.Route.TurnID
		}
		fact.Kind = executionapi.EventQuestion
		fact.EventFact.Questions = call.Questions
		fact.QuestionRoute = &route
	case "item/completed":
		if !inspectReply {
			return fact, nil
		}
		var params struct {
			ThreadID string `json:"threadId"`
			TurnID   string `json:"turnId"`
			Item     struct {
				ID           string `json:"id"`
				Type         string `json:"type"`
				Status       string `json:"status"`
				Success      *bool  `json:"success"`
				ContentItems []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"contentItems"`
			} `json:"item"`
		}
		if json.Unmarshal(envelope.Params, &params) != nil || params.ThreadID != threadID || params.TurnID != turnID {
			return fact, errors.New("app-server tool completion changed native identity")
		}
		if params.Item.Type != "dynamicToolCall" {
			return fact, nil
		}
		reply := executionapi.ToolReplyFact{ThreadID: params.ThreadID, TurnID: params.TurnID, CallID: params.Item.ID, Status: params.Item.Status, Success: params.Item.Success}
		for _, content := range params.Item.ContentItems {
			if content.Type != "inputText" {
				continue
			}
			var body executionapi.Answer
			if json.Unmarshal([]byte(content.Text), &body) == nil {
				reply.Answers = append(reply.Answers, body.Answers)
			}
		}
		fact.ToolReply = &reply
		fact.Kind = executionapi.EventCompleted
	case "turn/completed":
		var params struct {
			ThreadID string `json:"threadId"`
			Turn     struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		if json.Unmarshal(envelope.Params, &params) != nil || params.ThreadID != threadID || params.Turn.ID != turnID {
			return fact, errors.New("app-server turn completion changed its fixed identity")
		}
		fact.Kind = executionapi.EventCompleted
		fact.Completed = true
	}
	return fact, nil
}

func DecodeQuestions(arguments json.RawMessage) ([]executionapi.Question, error) {
	// Decode the native schema before converting to common values. Receiver
	// and Action belong to center-generated choices, never native tool input.
	var body struct {
		Questions []struct {
			ID            string `json:"id"`
			Header        string `json:"header"`
			Question      string `json:"question"`
			AllowFreeText bool   `json:"allow_free_text"`
			Options       []struct {
				ID     string `json:"id"`
				Label  string `json:"label"`
				Detail string `json:"detail"`
			} `json:"options"`
		} `json:"questions"`
	}
	err := json.Unmarshal(arguments, &body)
	if body.Questions == nil {
		return nil, err
	}
	questions := make([]executionapi.Question, len(body.Questions))
	for i, wire := range body.Questions {
		question := executionapi.Question{ID: wire.ID, Header: wire.Header, Question: wire.Question, AllowFreeText: wire.AllowFreeText}
		if wire.Options != nil {
			question.Options = make([]executionapi.DecisionOption, len(wire.Options))
			for j, option := range wire.Options {
				question.Options[j] = executionapi.DecisionOption{ID: option.ID, Label: option.Label, Detail: option.Detail}
			}
		}
		questions[i] = question
	}
	return questions, err
}
