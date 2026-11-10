package ports

import "context"

// ReadingRecord describes a field in an original persistent record. It carries
// no complete body and grants no execution capability.
type ReadingRecord struct {
	Seq                                                   int64
	ID, Role, Kind, Label, Preview, SHA256                string
	Bytes                                                 int
	At, State, RunID, AttemptID, SessionID, TurnID, Group string
}

type ReadingWindow struct {
	Records []ReadingRecord
	End     int64
	More    bool
}

type ActivityReadingReader interface {
	ReadReadingWindow(context.Context, string, string, string, int64, int64, int) (ReadingWindow, error)
	ReadingVersion(context.Context, string, string, string) (string, error)
	ReadingBodyIdentity(context.Context, string, string, string, string) (ReadingRecord, error)
	ReadReadingBody(context.Context, string, string, string, string, int) (string, error)
}
