package steerline

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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

// startDurableLocalWorker gives the child inherited file handles rather than a
// stdout pipe owned by the center. The exact process and journal can therefore
// be observed after a service restart without killing or replacing the session.
func startDurableLocalWorker(ctx context.Context, c Config, run *Run, turnID, prompt, events, stderr string, args []string) (*Worker, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if run == nil || run.WorktreePath == nil || run.AttemptID == "" || !stableExecutionID(turnID) {
		return nil, errors.New("durable worker has no fixed workspace or turn identity")
	}
	dir := filepath.Dir(events)
	if err := AtomicWrite(filepath.Join(dir, "prompt.txt"), []byte(prompt)); err != nil {
		return nil, err
	}
	stdout, err := os.OpenFile(events, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	defer stdout.Close()
	journal, sourceIdentity, err := openJournal(events)
	if err != nil {
		return nil, err
	}
	_ = journal.Close()
	marker := ConversationSourceMarker{Version: 1, ProjectID: c.identity, WorkflowID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, Backend: BackendLocal, Protocol: ProtocolLocalProcess, Agent: "codex", TargetID: run.Target.ID, Source: "codex-jsonl", SourceSegment: "local-jsonl-v1:" + sourceIdentity, SourceIdentity: sourceIdentity, RecordName: "events.jsonl", CreatedAt: now(), ReaderCommand: codexReaderCommand(c), CodexHome: codexEnvironmentHome()}
	if err := marker.validate(); err != nil {
		return nil, err
	}
	if err := AtomicWrite(filepath.Join(dir, ConversationSourceMarkerName), append(mustJSON(marker), '\n')); err != nil {
		return nil, err
	}
	stderrFile, err := os.OpenFile(stderr, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	defer stderrFile.Close()
	stdin, err := os.Open(filepath.Join(dir, "prompt.txt"))
	if err != nil {
		return nil, err
	}
	defer stdin.Close()
	cmd := exec.Command(c.Codex.Executable, args...)
	cmd.Dir = *run.WorktreePath
	cmd.Env = unsetEnv(overrideEnv(cmd.Environ(), map[string]string{"STEERLINE_WORKFLOW_RUN_ID": run.RunID, "STEERLINE_EXECUTION_ID": run.ExecutionID, "STEERLINE_WORKER_MESSAGE_ID": turnID}), []string{"BEADS_*", "BD_*"})
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderrFile
	identity, err := startDetachedProcess(cmd)
	if err != nil {
		return nil, err
	}
	observerCtx, cancel := context.WithCancel(context.Background())
	w := &Worker{proc: &durableWorkerProcess{identity: identity, process: cmd.Process}, Events: make(chan WorkerEvent, 64), Done: make(chan error, 1), cancelObservation: cancel, observationDone: make(chan struct{})}
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	go observeDurableWorker(observerCtx, w, events, sourceIdentity, run.RunID, run.AttemptID, waited)
	return w, nil
}

func observeDurableWorker(ctx context.Context, worker *Worker, path, sourceIdentity, runID, attemptID string, waited <-chan error) {
	observeDurableWorkerSegment(ctx, worker, path, sourceIdentity, "local-jsonl-v1:"+sourceIdentity, runID, attemptID, waited)
}

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
