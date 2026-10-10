package codexworker

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/erranliu/SteerLine/internal/execution/adapters/protocol/codexapp"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// AnswerEvidence confirms that the original native tool call completed with
// exactly the accepted answers recorded by the service writer.
func AnswerEvidence(path, threadID, turnID, callID string, answers map[string][]string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	expected, _ := json.Marshal(answers)
	decoder := json.NewDecoder(file)
	for {
		var event codexapp.Envelope
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				return false, nil
			}
			return false, err
		}
		if event.Method != "item/completed" {
			continue
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
		if err := json.Unmarshal(event.Params, &params); err != nil {
			return false, err
		}
		if params.ThreadID != threadID || params.TurnID != turnID || params.Item.ID != callID || params.Item.Type != "dynamicToolCall" {
			continue
		}
		if params.Item.Status != "completed" || params.Item.Success == nil || !*params.Item.Success {
			return false, nil
		}
		for _, item := range params.Item.ContentItems {
			if item.Type != "inputText" {
				continue
			}
			var body executionbackend.Answer
			if json.Unmarshal([]byte(item.Text), &body) == nil {
				actual, _ := json.Marshal(body.Answers)
				if bytes.Equal(actual, expected) {
					return true, nil
				}
			}
		}
		return false, nil
	}
}

type AskJournal struct {
	Calls     []codexapp.Envelope
	Completed map[string]bool
}

// ReadAskJournal scans native ask events in order. The domain validator keeps
// application Decision rules out of this protocol package.
func ReadAskJournal(path, threadID, turnID string, validate func(codexapp.Envelope) error) (AskJournal, error) {
	file, err := os.Open(path)
	if err != nil {
		return AskJournal{}, err
	}
	defer file.Close()
	journal := AskJournal{Calls: make([]codexapp.Envelope, 0), Completed: map[string]bool{}}
	decoder := json.NewDecoder(file)
	for {
		var event codexapp.Envelope
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				return journal, nil
			}
			return AskJournal{}, err
		}
		switch event.Method {
		case "item/tool/call":
			if err := validate(event); err != nil {
				return AskJournal{}, err
			}
			journal.Calls = append(journal.Calls, event)
		case "item/completed":
			if IsAsyncQuestion(event) {
				if err := validate(event); err != nil {
					return AskJournal{}, err
				}
				journal.Calls = append(journal.Calls, event)
				continue
			}
			var params struct {
				ThreadID string `json:"threadId"`
				TurnID   string `json:"turnId"`
				Item     struct {
					ID   string `json:"id"`
					Type string `json:"type"`
				} `json:"item"`
			}
			if json.Unmarshal(event.Params, &params) != nil {
				return AskJournal{}, errors.New("app-server journal has invalid completed item")
			}
			if params.ThreadID == threadID && params.TurnID == turnID && params.Item.Type == "dynamicToolCall" {
				journal.Completed[params.Item.ID] = true
			}
		}
	}
}
