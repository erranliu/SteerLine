package ports

import (
	"context"
	"encoding/json"
)

// AsyncActivityFact contains only durable domain facts. SQL ownership stays in
// storage; state interpretation stays with the product query.
type AsyncActivityFact struct {
	Source  string
	ID      string
	SortAt  string
	Payload json.RawMessage
	Related json.RawMessage
}
type AsyncActivityRead struct {
	Source     string
	Filter     string
	AfterTime  string
	AfterID    string
	Limit      int
	RequestIDs []string
	SourceID   string
}
type AsyncActivityReader interface {
	// Implementations must honor context during pool waits, SQL and decoding.
	// Filter pruning must be a superset of final product projection matches.
	ReadAsyncActivityFacts(context.Context, string, AsyncActivityRead) ([]AsyncActivityFact, bool, error)
}
