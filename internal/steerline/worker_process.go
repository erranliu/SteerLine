package steerline

import (
	"bytes"
	"context"
	"errors"
	"os"
	"time"
)

type durableWorkerProcess struct {
	identity ProcessIdentity
	process  *os.Process
}

func (p *durableWorkerProcess) Identity() ProcessIdentity { return p.identity }
func (p *durableWorkerProcess) Kill() {
	if p.process != nil {
		_ = p.process.Kill()
	}
}
func (p *durableWorkerProcess) Close() {} // Closing observation never kills the worker.

func observeDurableWorkerSegment(ctx context.Context, worker *Worker, path, sourceIdentity, segment, runID, attemptID string, waited <-chan error) {
	defer close(worker.observationDone)
	defer close(worker.Events)
	cursor := JournalCursor{}
	var seq uint64
	var processErr error
	exited := false
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		batch, err := ReadJournalBatch(path, cursor, JournalOptions{FromStart: true, KeepCR: true})
		if err != nil {
			worker.Done <- err
			return
		}
		if cursor.Initialized && batch.Reset {
			worker.Done <- errors.New("durable worker journal identity changed")
			return
		}
		cursor = batch.Cursor
		for _, record := range batch.Records {
			seq++
			if record.Oversized {
				if !deliverDurableWorkerEvent(ctx, worker, WorkerEvent{RunID: runID, AttemptID: attemptID, Source: "codex-jsonl", SourceSegment: segment, SourceIdentity: sourceIdentity, SourceSeq: seq, SourceOffset: record.Offset, SourceLength: record.Length, SourceGap: true, Diagnostic: "oversized worker event", Activity: ActivityEvent{Source: "codex", Kind: "unknown", Title: "oversized worker event"}}) {
					return
				}
				continue
			}
			line := bytes.TrimSuffix(record.Data, []byte{'\r'})
			raw := append(append([]byte(nil), record.Data...), '\n')
			accepted := true
			emitWorkerLine(line, raw, NewCodexNormalizer(), seq, record.Offset, record.Length, func(event WorkerEvent) {
				event.RunID, event.AttemptID, event.Source = runID, attemptID, "codex-jsonl"
				event.SourceSegment, event.SourceIdentity = segment, sourceIdentity
				if accepted {
					accepted = deliverDurableWorkerEvent(ctx, worker, event)
				}
			})
			if !accepted {
				return
			}
		}
		if exited && len(batch.Records) == 0 && !batch.More {
			worker.Done <- processErr
			return
		}
		select {
		case <-ctx.Done():
			return
		case processErr = <-waited:
			exited = true
		case <-ticker.C:
		}
	}
}

func deliverDurableWorkerEvent(ctx context.Context, worker *Worker, event WorkerEvent) bool {
	event.Persisted = make(chan error, 1)
	select {
	case worker.Events <- event:
	case <-ctx.Done():
		return false
	}
	select {
	case err := <-event.Persisted:
		return err == nil
	case <-ctx.Done():
		return false
	}
}
