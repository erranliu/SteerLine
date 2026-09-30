package codexworker

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

type Outcome struct {
	SessionID    string
	NativeStatus string
	Started      bool
	Completed    bool
	Succeeded    bool
	LastMessage  string
	Failure      string
	PRCandidates []string
}

func ReadAppServerOutcome(path, threadID, turnID string) (Outcome, error) {
	var outcome Outcome
	seenPR := map[string]bool{}
	addPR := func(message string) {
		for _, candidate := range executionapi.PRClues(message) {
			if !seenPR[candidate] {
				seenPR[candidate] = true
				outcome.PRCandidates = append(outcome.PRCandidates, candidate)
			}
		}
	}
	file, err := os.Open(path)
	if err != nil {
		return outcome, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	for decoder.More() {
		var event codexapp.Envelope
		if err := decoder.Decode(&event); err != nil {
			return outcome, err
		}
		switch event.Method {
		case "item/completed":
			var p struct {
				ThreadID string `json:"threadId"`
				TurnID   string `json:"turnId"`
				Item     struct {
					Type  string `json:"type"`
					Text  string `json:"text"`
					Phase string `json:"phase"`
				} `json:"item"`
			}
			if json.Unmarshal(event.Params, &p) != nil || p.ThreadID != threadID || p.TurnID != turnID {
				return outcome, errors.New("app-server item completion identity mismatch")
			}
			if p.Item.Type == "agentMessage" {
				addPR(p.Item.Text)
				if p.Item.Phase == "final_answer" {
					outcome.LastMessage = p.Item.Text
				}
			}
		case "turn/started":
			var p struct {
				ThreadID string `json:"threadId"`
				Turn     struct {
					ID string `json:"id"`
				} `json:"turn"`
			}
			if json.Unmarshal(event.Params, &p) != nil || p.ThreadID != threadID || p.Turn.ID != turnID {
				return outcome, errors.New("app-server turn start identity mismatch")
			}
			outcome.Started = true
			outcome.SessionID = threadID
		case "turn/completed":
			var p struct {
				ThreadID string `json:"threadId"`
				Turn     struct {
					ID     string          `json:"id"`
					Status string          `json:"status"`
					Error  json.RawMessage `json:"error"`
					Items  []struct {
						Type  string `json:"type"`
						Text  string `json:"text"`
						Phase string `json:"phase"`
					} `json:"items"`
				} `json:"turn"`
			}
			if json.Unmarshal(event.Params, &p) != nil || p.ThreadID != threadID || p.Turn.ID != turnID {
				return outcome, errors.New("app-server turn completion identity mismatch")
			}
			outcome.SessionID = threadID
			outcome.Completed = true
			outcome.NativeStatus = p.Turn.Status
			outcome.Succeeded = p.Turn.Status == "completed"
			if p.Turn.Status != "completed" {
				outcome.Failure = "native turn ended with status " + p.Turn.Status
			}
			for _, item := range p.Turn.Items {
				if item.Type == "agentMessage" {
					addPR(item.Text)
					if item.Phase == "final_answer" {
						outcome.LastMessage = item.Text
					}
				}
			}
		}
	}
	return outcome, nil
}
