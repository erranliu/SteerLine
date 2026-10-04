package sqlite

import (
	"errors"
)

// A duplicate rolls back its read-only transaction so database and graph
// revisions, receipts, and event cursors remain exactly as first committed.
var errAtomicReplay = errors.New("original atomic receipt already committed")
