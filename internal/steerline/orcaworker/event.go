package orcaworker

import (
	"encoding/json"
	"strings"
)

// MessageFact belongs to one verified Dispatch. It carries no business state.
type MessageFact struct {
	Kind       string
	MessageID  string
	TaskID     string
	DispatchID string
	Question   *Question
	Outcome    string
}

// MessagePayload unwraps the JSON string used by the native inbox. Some
// surfaces return the structured payload directly.
func MessagePayload(message Message) []byte {
	if len(message.Payload) == 0 || string(message.Payload) == "null" {
		return nil
	}
	var text string
	if json.Unmarshal(message.Payload, &text) == nil {
		return []byte(text)
	}
	return append([]byte(nil), message.Payload...)
}

func DecodeMessage(message Message, taskID, dispatchID, agentTerminal string) (MessageFact, bool, error) {
	var fact MessageFact
	payload := MessagePayload(message)
	if !MessageMatches(message.FromHandle, message.Type, payload, taskID, dispatchID, agentTerminal) {
		return fact, false, nil
	}
	fact.Kind, fact.MessageID, fact.TaskID, fact.DispatchID = message.Type, message.ID, taskID, dispatchID
	switch message.Type {
	case "question":
		question := ParseQuestion(payload)
		if strings.TrimSpace(question.Text) == "" {
			question.Text = strings.TrimSpace(message.Body)
			if question.Text == "" {
				question.Text = strings.TrimSpace(message.Subject)
			}
		}
		fact.Question = &question
	case "worker_done":
		outcome, err := DoneOutcome(payload, taskID, dispatchID)
		if err != nil {
			return fact, true, err
		}
		fact.Outcome = outcome
	}
	return fact, true, nil
}
