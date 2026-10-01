package contracts

const MaxConversationBodyBytes = 16 << 20

type ConversationFilter struct {
	WorkflowID  string `json:"workflow_id,omitempty"`
	ExecutionID string `json:"execution_id,omitempty"`
	AttemptID   string `json:"attempt_id,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	TurnID      string `json:"turn_id,omitempty"`
	Source      string `json:"source,omitempty"`
}

type ConversationPage struct {
	Events       []ConversationEvent `json:"events"`
	NextCursor   string              `json:"next_cursor,omitempty"`
	HasMore      bool                `json:"has_more"`
	PayloadBytes int                 `json:"payload_bytes"`
}

type ConversationBodyPage struct {
	ItemKey      string           `json:"item_key"`
	Revision     int64            `json:"revision"`
	Offset       int64            `json:"offset"`
	NextOffset   int64            `json:"next_offset"`
	TotalBytes   int64            `json:"total_bytes"`
	SHA256       string           `json:"sha256"`
	Completeness BodyCompleteness `json:"completeness"`
	Complete     bool             `json:"complete"`
	Data         []byte           `json:"data"`
}

type ConversationRawPage struct {
	RawID      string `json:"raw_id"`
	Offset     int64  `json:"offset"`
	NextOffset int64  `json:"next_offset"`
	TotalBytes int64  `json:"total_bytes"`
	SHA256     string `json:"sha256"`
	Complete   bool   `json:"complete"`
	Data       []byte `json:"data"`
}

// ConversationEventRecord preserves stored bytes until the query page budget allows decoding.
type ConversationEventRecord struct {
	EventID      int64
	PayloadBytes int
	Payload      []byte
}

// ConversationBodyRecord is one bounded, immutable message revision from storage.
type ConversationBodyRecord struct {
	Body         []byte
	TotalBytes   int64
	SHA256       string
	Completeness BodyCompleteness
}

type ConversationRawRecord struct {
	Data       []byte
	TotalBytes int64
	SHA256     string
}
