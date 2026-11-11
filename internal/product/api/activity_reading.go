package api

// ReadingPreview is a bounded observation, never evidence of delivery or success.
type ReadingPreview struct {
	Kind       string `json:"kind"`
	Text       string `json:"text"`
	At         string `json:"at,omitempty"`
	ObservedAt string `json:"observed_at,omitempty"`
	Truncated  bool   `json:"truncated"`
	Diagnostic string `json:"diagnostic,omitempty"`
}

type ReadingItem struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	Preview   string `json:"preview"`
	Bytes     int    `json:"bytes"`
	SHA256    string `json:"sha256"`
	At        string `json:"at,omitempty"`
	State     string `json:"state,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	AttemptID string `json:"attempt_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	TurnID    string `json:"turn_id,omitempty"`
	Group     string `json:"group"`
}

type ReadingPage struct {
	ActivityID string        `json:"activity_id"`
	Items      []ReadingItem `json:"items"`
	NextCursor string        `json:"next_cursor,omitempty"`
	HasMore    bool          `json:"has_more"`
	Version    string        `json:"version"`
	Diagnostic string        `json:"diagnostic"`
}

type ReadingBody struct {
	ID         string `json:"id"`
	SHA256     string `json:"sha256"`
	Offset     int    `json:"offset"`
	NextOffset int    `json:"next_offset"`
	Bytes      int    `json:"bytes"`
	Text       string `json:"text"`
	Complete   bool   `json:"complete"`
}
