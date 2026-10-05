package codexworker

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/erranliu/SteerLine/internal/execution/adapters/protocol/codexapp"
)

type ObservationEvent struct {
	Persisted chan error
	Summary   string
	Envelope  codexapp.Envelope
	Forward   bool
	Kind      string
	Status    string
	Title     string
}

// Observe journals native notifications before reporting them to the service.
// The deliver callback must not return true until the event is durable.
func Observe(ctx context.Context, client *codexapp.Client, journal *os.File, threadID, turnID string, replay []codexapp.Envelope, validateAsk func(codexapp.Envelope) error, deliver func(ObservationEvent) bool) (completed bool, err error) {
	for _, envelope := range replay {
		if !deliver(ObservationEvent{Envelope: envelope, Summary: envelope.Method, Forward: true, Kind: "question", Status: "pending", Title: "Codex needs a decision"}) {
			return false, nil
		}
	}
	for {
		select {
		case <-ctx.Done():
			return false, nil
		case envelope, ok := <-client.Notifications():
			if !ok {
				return false, errors.New("app-server observation ended before native turn completion")
			}
			line, err := json.Marshal(envelope)
			if err != nil {
				return false, err
			}
			if _, err := journal.Write(append(line, '\n')); err != nil {
				return false, err
			}
			if err := journal.Sync(); err != nil {
				return false, err
			}
			event := ObservationEvent{Envelope: envelope, Summary: envelope.Method, Forward: true}
			if envelope.Method == "item/tool/call" {
				if err := validateAsk(envelope); err != nil {
					if rejectErr := client.RejectRequest(envelope.ID, "SteerLine rejected an unsupported or malformed native question"); rejectErr != nil {
						return false, errors.Join(err, rejectErr)
					}
					event.Forward = false
					event.Kind, event.Status, event.Title = "question", "rejected", "Unsupported Codex question"
					if !deliver(event) {
						if ctx.Err() == nil {
							return false, errors.New("unsupported Codex question could not be recorded")
						}
						return false, nil
					}
					continue
				}
			}
			if len(envelope.ID) != 0 && envelope.Method != "item/tool/call" {
				if err := client.RejectRequest(envelope.ID, "SteerLine cannot authorize this native Codex host request"); err != nil {
					return false, err
				}
				event.Kind, event.Status, event.Title = "approval", "rejected", "Unsupported Codex host request"
				if !deliver(event) {
					if ctx.Err() == nil {
						return false, errors.New("unsupported Codex host request could not be recorded")
					}
					return false, nil
				}
				continue
			}
			switch envelope.Method {
			case "turn/started":
				event.Kind, event.Status, event.Title = "turn", "started", "Codex turn started"
			case "item/tool/call":
				event.Kind, event.Status, event.Title = "question", "pending", "Codex needs a decision"
			case "item/completed":
				var params struct {
					Item struct {
						Type string `json:"type"`
					} `json:"item"`
				}
				if json.Unmarshal(envelope.Params, &params) != nil || params.Item.Type != "dynamicToolCall" {
					continue
				}
				event.Kind, event.Status, event.Title = "question", "completed", "Codex decision tool completed"
			case "turn/completed":
				event.Kind, event.Status, event.Title = "turn", "completed", "Codex turn completed"
			default:
				continue
			}
			if !deliver(event) {
				if ctx.Err() == nil {
					return false, errors.New("Codex app-server event could not be committed")
				}
				return false, nil
			}
			if envelope.Method == "turn/completed" {
				return true, nil
			}
		}
	}
}
