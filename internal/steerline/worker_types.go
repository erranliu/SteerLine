package steerline

import (
	"context"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

type WorkerEvent struct {
	RunID          string
	AttemptID      string
	Source         string
	SourceEventID  string
	SourceIdentity string
	SourceCursor   string
	Session        string
	Summary        string
	Activity       ActivityEvent
	Raw            []byte
	SourceSeq      uint64
	SourceOffset   int64
	SourceLength   int64
	SourceSegment  string
	SourceGap      bool
	SourceChanged  bool
	InputWait      bool
	Diagnostic     string
	Fact           executionapi.EventFact
	Persisted      chan error
	OutputSource   string
	OutputCursor   string
	OutputGap      bool
	TurnStarted    bool
	Processed      chan error `json:"-"`
}

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
