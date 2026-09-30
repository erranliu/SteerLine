package codexworker

import (
	"encoding/json"
	"errors"
	"reflect"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
)

type ToolReplyFact struct {
	ThreadID, TurnID, CallID, Status string
	Success                          *bool
	Answers                          []map[string][]string
}

func (f ToolReplyFact) MatchesAnswers(expected map[string][]string) bool {
	for _, answers := range f.Answers {
		if reflect.DeepEqual(answers, expected) {
			return true
		}
	}
	return false
}

type EventFact struct {
	StartedTurnID string
	Question      *AskEnvelope
	ToolReply     *ToolReplyFact
}

// DecodeEvent checks native identities before exposing facts to the center.
func DecodeEvent(envelope codexapp.Envelope, threadID, turnID string, inspectReply bool) (EventFact, error) {
	var fact EventFact
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
	case "item/tool/call":
		call, err := ParseAskEnvelope(envelope, threadID, turnID)
		if err != nil {
			return fact, err
		}
		fact.Question = &call
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
		reply := ToolReplyFact{ThreadID: params.ThreadID, TurnID: params.TurnID, CallID: params.Item.ID, Status: params.Item.Status, Success: params.Item.Success}
		for _, content := range params.Item.ContentItems {
			if content.Type != "inputText" {
				continue
			}
			var body struct {
				Answers map[string][]string `json:"answers"`
			}
			if json.Unmarshal([]byte(content.Text), &body) == nil {
				reply.Answers = append(reply.Answers, body.Answers)
			}
		}
		fact.ToolReply = &reply
	}
	return fact, nil
}

type AskOption struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
}
type AskQuestion struct {
	ID            string      `json:"id"`
	Header        string      `json:"header"`
	Question      string      `json:"question"`
	Options       []AskOption `json:"options"`
	AllowFreeText bool        `json:"allow_free_text"`
}

func DecodeQuestions(arguments json.RawMessage) ([]AskQuestion, error) {
	var body struct {
		Questions []AskQuestion `json:"questions"`
	}
	err := json.Unmarshal(arguments, &body)
	return body.Questions, err
}
