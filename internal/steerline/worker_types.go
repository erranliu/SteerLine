package steerline

import (
	"context"
	"time"

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
	Implementation []executionapi.ImplementationDataEnvelope
	Candidate      *executionapi.LifecycleCandidate
	DeliveryID     string
	DeliveryFinal  bool
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

func workerFromLifecycle(runID, attemptID string, observer executionapi.LifecycleObserver) *Worker {
	if observer == nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &Worker{
		proc: observer.StopHandle(), Events: make(chan WorkerEvent, 64), Done: make(chan error, 1),
		observationDone: make(chan struct{}),
		cancelObservation: func() {
			cancel()
			closeCtx, closeCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer closeCancel()
			_ = observer.Close(closeCtx)
		},
	}
	go func() {
		defer close(w.observationDone)
		defer close(w.Events)
		defer close(w.Done)
		for value := range observer.Events() {
			value = value.Clone()
			event := WorkerEvent{
				RunID: runID, AttemptID: attemptID, Source: "execution", Summary: value.Summary,
				Activity: ActivityEvent{Source: "execution", Kind: value.Kind, Status: value.Status, Title: value.Title},
				Fact:     value.Fact, Implementation: cloneImplementationData(value.Implementation),
				Candidate:  value.Candidate,
				DeliveryID: value.DeliveryID, DeliveryFinal: value.DeliveryFinal,
				Persisted: value.Persisted, Processed: value.Processed,
			}
			select {
			case w.Events <- event:
			case <-ctx.Done():
				ackWorkerEventPersistence(event, ctx.Err())
				return
			}
		}
		select {
		case err, ok := <-observer.Done():
			if ok {
				w.Done <- err
			}
		case <-ctx.Done():
		}
	}()
	return w
}

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
