package query

import (
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

// ActivityDisplayEvent selects only the canonical activity fields consumed by
// the terminal formatter. Unrecognized event payload bytes remain in storage.
func ActivityDisplayEvent(event contracts.ActivityEvent) api.ActivityEvent {
	metadata := make(map[string]any, len(event.Meta))
	for key, value := range event.Meta {
		metadata[key] = value
	}
	return api.ActivityEvent{V: event.V, Seq: event.Seq, TS: event.TS, Source: event.Source, RunID: event.RunID, TaskID: event.TaskID, ItemID: event.ItemID, Kind: event.Kind, Status: event.Status, Title: event.Title, Detail: event.Detail, ElapsedMS: event.ElapsedMS, Meta: metadata}
}
