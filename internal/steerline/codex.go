package steerline

import (
	"context"
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
	Persisted      chan error
}
type Worker struct {
	proc              process
	Events            chan WorkerEvent
	Done              chan error
	cancelObservation context.CancelFunc
	observationDone   chan struct{}
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
