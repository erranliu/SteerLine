package contracts

import (
	"encoding/json"
	"time"
)

type ActivityEvent struct {
	V         int             `json:"v"`
	Seq       uint64          `json:"seq"`
	TS        time.Time       `json:"ts"`
	Source    string          `json:"source"`
	RunID     string          `json:"run_id,omitempty"`
	TaskID    string          `json:"task_id,omitempty"`
	ItemID    string          `json:"item_id,omitempty"`
	Kind      string          `json:"kind"`
	Status    string          `json:"status,omitempty"`
	Title     string          `json:"title,omitempty"`
	Detail    string          `json:"detail,omitempty"`
	ElapsedMS int64           `json:"elapsed_ms,omitempty"`
	Meta      map[string]any  `json:"meta,omitempty"`
	Raw       json.RawMessage `json:"raw,omitempty"`
}
