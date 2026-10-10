package orcaworker

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type Message struct {
	ID         string          `json:"id"`
	RunID      string          `json:"run_id,omitempty"`
	ToHandle   string          `json:"to_handle,omitempty"`
	FromHandle string          `json:"from_handle,omitempty"`
	Type       string          `json:"type"`
	Subject    string          `json:"subject"`
	Body       string          `json:"body"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	CreatedAt  time.Time       `json:"created_at,omitempty"`
}

type QuestionOption struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
}

type Question struct {
	Text    string
	Options []QuestionOption
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

// ParseQuestion extracts Orca's optional structured question. The service
// supplies the human-facing fallback text and persists the Decision.
func ParseQuestion(payload []byte) Question {
	var body struct {
		Question string            `json:"question"`
		Options  []json.RawMessage `json:"options"`
	}
	if json.Unmarshal(payload, &body) != nil {
		return Question{}
	}
	result := Question{Text: body.Question}
	for i, raw := range body.Options {
		var label string
		if json.Unmarshal(raw, &label) == nil {
			if label != "" {
				result.Options = append(result.Options, QuestionOption{ID: fmt.Sprintf("option-%d", i+1), Label: label})
			}
			continue
		}
		var option QuestionOption
		if json.Unmarshal(raw, &option) != nil {
			continue
		}
		if option.ID == "" {
			option.ID = fmt.Sprintf("option-%d", i+1)
		}
		if option.Label != "" {
			result.Options = append(result.Options, option)
		}
	}
	return result
}

func MessageMatches(fromHandle, messageType string, payload []byte, taskID, dispatchID, agentTerminal string) bool {
	if fromHandle != "dispatch:"+dispatchID && fromHandle != agentTerminal {
		return false
	}
	if len(payload) == 0 && (messageType == "status" || messageType == "escalation") && fromHandle == "dispatch:"+dispatchID {
		return true
	}
	var identity struct {
		TaskID     string `json:"taskId"`
		DispatchID string `json:"dispatchId"`
	}
	return json.Unmarshal(payload, &identity) == nil && identity.TaskID == taskID && identity.DispatchID == dispatchID
}

func DoneOutcome(payload []byte, taskID, dispatchID string) (string, error) {
	var result struct {
		TaskID     string `json:"taskId"`
		DispatchID string `json:"dispatchId"`
		Outcome    string `json:"outcome"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return "", err
	}
	if result.TaskID != taskID || result.DispatchID != dispatchID || result.Outcome != "succeeded" && result.Outcome != "failed" {
		return "", errors.New("worker_done does not match the current Orca Task/Dispatch")
	}
	return result.Outcome, nil
}
