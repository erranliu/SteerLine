package steerline

import (
	"context"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
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

type OrcaInboxMessage = orcaworker.Message

type Worker struct {
	proc              executionapi.StopHandle
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
