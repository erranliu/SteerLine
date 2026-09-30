package steerline

import (
	"context"
	"encoding/json"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
)

type WorkerEvent struct {
	AppServer        *codexapp.Envelope
	RunID            string
	AttemptID        string
	Source           string
	SourceEventID    string
	SourceIdentity   string
	SourceCursor     string
	Session          string
	Summary          string
	Activity         ActivityEvent
	Raw              []byte
	SourceSeq        uint64
	SourceOffset     int64
	SourceLength     int64
	SourceSegment    string
	SourceGap        bool
	SourceChanged    bool
	InputWait        bool
	Diagnostic       string
	Persisted        chan error
	OutputSource     string
	OutputCursor     string
	OutputGap        bool
	TurnStarted      bool
	RemoteRunID      string
	RemoteDeliveryID string
	RemoteMessages   []OrcaInboxMessage
	Acknowledge      func() (string, error) `json:"-"`
	Processed        chan error             `json:"-"`
}

type OrcaInboxMessage struct {
	ID         string          `json:"id"`
	FromHandle string          `json:"from_handle,omitempty"`
	Type       string          `json:"type"`
	Subject    string          `json:"subject"`
	Body       string          `json:"body"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	CreatedAt  time.Time       `json:"created_at,omitempty"`
}
type Worker struct {
	proc              process
	Events            chan WorkerEvent
	Done              chan error
	cancelObservation context.CancelFunc
	observationDone   chan struct{}
	completionAck     func() error
	RemoteOutcome     string
}

func (w *Worker) Kill()  { w.proc.Kill() }
func (w *Worker) Close() { w.proc.Close() }
func (w *Worker) detachLocalObservation(ctx context.Context) error {
	if w == nil || w.cancelObservation == nil || w.observationDone == nil {
		return unsupported("local observation detach")
	}
	w.cancelObservation()
	select {
	case <-w.observationDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
