package codexworker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/erranliu/SteerLine/internal/execution/adapters/protocol/codexapp"
)

// SteerAsyncAnswer sends one saved input to the original active turn. A failed
// precondition or uncertain receipt never falls back to starting another turn.
func SteerAsyncAnswer(ctx context.Context, client *codexapp.Client, thread, workspace, turn, command, message string) error {
	if err := ResumeThread(ctx, client, thread, workspace); err != nil {
		return err
	}
	var receipt struct {
		TurnID string `json:"turnId"`
	}
	if err := client.Call(ctx, "turn/steer", map[string]any{
		"threadId": thread, "expectedTurnId": turn, "clientUserMessageId": command,
		"input": []map[string]string{{"type": "text", "text": message}},
	}, &receipt); err != nil {
		return err
	}
	if receipt.TurnID != turn {
		return errors.New("async answer receipt changed the original turn")
	}
	return nil
}

func asyncAnswerItem(raw json.RawMessage, command, message string) bool {
	var item struct {
		Type     string `json:"type"`
		ClientID string `json:"clientId"`
		Content  []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &item) != nil || item.Type != "userMessage" || item.ClientID != command || len(item.Content) != 1 {
		return false
	}
	return item.Content[0].Type == "text" && item.Content[0].Text == message
}

// Reconciliation reads the durable observation journal; it never resends.
func AsyncAnswerEvidence(path, thread, turn, command, message string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	for {
		var e codexapp.Envelope
		if err := decoder.Decode(&e); err != nil {
			if errors.Is(err, io.EOF) {
				return false, nil
			}
			return false, err
		}
		if e.Method != "item/completed" {
			continue
		}
		var p struct {
			ThreadID string          `json:"threadId"`
			TurnID   string          `json:"turnId"`
			Item     json.RawMessage `json:"item"`
		}
		if json.Unmarshal(e.Params, &p) == nil && p.ThreadID == thread && p.TurnID == turn && asyncAnswerItem(p.Item, command, message) {
			return true, nil
		}
	}
}
