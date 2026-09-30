package codexworker

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
)

// EventFact carries verified native evidence without business records.
type EventFact struct {
	StartedTurnID string
	Ask           *AskEnvelope
	ToolReply     *ToolReply
}

type ToolReply struct {
	ThreadID string
	TurnID   string
	CallID   string
	Status   string
	Success  *bool
	Answers  []map[string][]string
}

func (f ToolReply) MatchesAnswers(expected map[string][]string) bool {
	want, _ := json.Marshal(expected)
	for _, answers := range f.Answers {
		got, _ := json.Marshal(answers)
		if bytes.Equal(got, want) {
			return true
		}
	}
	return false
}

// DecodeEvent checks the fixed thread/turn before exposing a native fact.
// Ask argument content is validated by the center's Decision rules.
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
		ask, err := ParseAskEnvelope(envelope, threadID, turnID)
		if err != nil {
			return fact, err
		}
		fact.Ask = &ask
	case "item/completed":
		if !inspectReply {
			break
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
			break
		}
		reply := ToolReply{ThreadID: params.ThreadID, TurnID: params.TurnID, CallID: params.Item.ID, Status: params.Item.Status, Success: params.Item.Success}
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
