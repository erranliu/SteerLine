package codexworker

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/erranliu/SteerLine/internal/execution/adapters/protocol/codexapp"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type asyncQuestionParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	Item     struct {
		ID        string `json:"id"`
		Type      string `json:"type"`
		Delivery  string `json:"delivery"`
		Questions []struct {
			Title   string   `json:"title"`
			Options []string `json:"options"`
		} `json:"questions"`
	} `json:"item"`
}

func IsAsyncQuestion(envelope codexapp.Envelope) bool {
	if envelope.Method != "item/completed" {
		return false
	}
	var p asyncQuestionParams
	return json.Unmarshal(envelope.Params, &p) == nil && p.Item.Type == "agentMessage" && p.Item.Delivery == "async" && len(p.Item.Questions) > 0
}

func ParseAsyncAsk(envelope codexapp.Envelope, threadID, turnID string) (AskCall, error) {
	var p asyncQuestionParams
	if !IsAsyncQuestion(envelope) || json.Unmarshal(envelope.Params, &p) != nil || p.ThreadID != threadID || p.TurnID != turnID || p.Item.ID == "" || !ValidNativeID(threadID) || !ValidNativeID(turnID) {
		return AskCall{}, errors.New("async question differs from its fixed native item, thread or turn")
	}
	questions := make([]executionbackend.Question, len(p.Item.Questions))
	for i, q := range p.Item.Questions {
		questions[i] = executionbackend.Question{ID: fmt.Sprintf("q%d", i+1), Header: fmt.Sprintf("Question %d", i+1), Question: q.Title, AllowFreeText: true}
		for j, label := range q.Options {
			questions[i].Options = append(questions[i].Options, executionbackend.DecisionOption{ID: fmt.Sprintf("q%d-o%d", i+1, j+1), Label: label})
		}
	}
	arguments, err := json.Marshal(map[string]any{"questions": questions})
	if err != nil {
		return AskCall{}, err
	}
	call, err := decodeAskCall(AskEnvelope{ThreadID: threadID, TurnID: turnID, CallID: p.Item.ID, Arguments: arguments})
	call.Route.Delivery = "async"
	return call, err
}
