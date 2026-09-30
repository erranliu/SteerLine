package codexworker

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

const maxJournalRecord = 8 << 20

type Outcome struct {
	SessionID    string
	NativeStatus string
	Started      bool
	Completed    bool
	LastMessage  string
	Failure      string
	PRCandidates []string
}

func ReadExecOutcome(eventsPath string) (Outcome, error) {
	var outcome Outcome
	seenPR := map[string]bool{}
	addPRClues := func(message string) {
		for _, url := range executionapi.PRClues(message) {
			if !seenPR[url] {
				seenPR[url] = true
				outcome.PRCandidates = append(outcome.PRCandidates, url)
			}
		}
	}
	file, err := os.Open(eventsPath)
	if err != nil {
		return outcome, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), maxJournalRecord)
	for lines := 0; scanner.Scan(); lines++ {
		if lines >= 100000 {
			return outcome, errors.New("worker turn journal exceeds its observation limit")
		}
		var event struct {
			Type     string `json:"type"`
			ThreadID string `json:"thread_id"`
			Error    struct {
				Message string `json:"message"`
			} `json:"error"`
			Item struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return outcome, fmt.Errorf("worker turn journal has unreadable output: %w", err)
		}
		switch event.Type {
		case "thread.started":
			if outcome.SessionID != "" && outcome.SessionID != event.ThreadID {
				return outcome, errors.New("worker turn changed native session identity")
			}
			outcome.SessionID = event.ThreadID
		case "turn.started":
			outcome.Started = true
		case "turn.completed":
			outcome.Completed = true
			outcome.NativeStatus = "completed"
		case "turn.failed", "error":
			outcome.Failure = event.Error.Message
		case "item.completed":
			if event.Item.Type == "agent_message" && event.Item.Text != "" {
				outcome.LastMessage = event.Item.Text
				addPRClues(event.Item.Text)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return outcome, err
	}
	lastPath := filepath.Join(filepath.Dir(eventsPath), "last-message.txt")
	if info, err := os.Stat(lastPath); err == nil && info.Size() <= 1<<20 {
		if data, err := os.ReadFile(lastPath); err == nil && strings.TrimSpace(string(data)) != "" {
			outcome.LastMessage = string(data)
			addPRClues(outcome.LastMessage)
		}
	}
	return outcome, nil
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
