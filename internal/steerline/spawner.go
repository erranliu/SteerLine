package steerline

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

// SpawnSpec is everything a host needs to run one worker. It contains no
// scheduling or recovery authority: the scheduler keeps ownership.
type SpawnSpec struct {
	RunID              string
	AttemptID          string
	ProjectID          string
	WorkflowID         string
	ExecutionID        string
	Agent              string
	TargetID           string
	ConversationJSONL  bool
	ConversationReader Command
	SourceIdentity     string
	Dir                string
	EventsPath         string
	StderrPath         string
	OutputPath         string
	Command            Command
	Stdin              io.Reader
	Env                map[string]string
	UnsetEnv           []string
	Timeout            time.Duration
}

// Spawner starts and supervises one worker. Implementations must not mutate
// scheduler state and must keep events.jsonl authoritative.
type Spawner interface {
	Host() string
	Start(ctx context.Context, spec SpawnSpec) (*Worker, error)
}

// SpawnerFor selects the process host for a locally-prepared workspace. The
// Orca host owns workspace preparation and agent launch itself (see
// orcaexec.go), so only the local host goes through this interface today.
func SpawnerFor(c Config) Spawner {
	return LocalSpawner{}
}

// LocalSpawner runs the worker as a detached OS child with a Job Object (Windows)
// or process group (Linux). This preserves the pre-P1 behavior exactly.
type LocalSpawner struct{}

const maxWorkerOutputBytes = 2 << 20

type maxOutputWriter struct {
	writer    io.Writer
	remaining int64
}

func (w *maxOutputWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > w.remaining {
		return 0, fmt.Errorf("worker output exceeds %d bytes", maxWorkerOutputBytes)
	}
	n, err := w.writer.Write(data)
	w.remaining -= int64(n)
	return n, err
}

func (LocalSpawner) Host() string { return "local" }

func (LocalSpawner) Start(ctx context.Context, spec SpawnSpec) (*Worker, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	logFile, e := os.OpenFile(spec.EventsPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return nil, e
	}
	identityFile, sourceIdentity, identityErr := openJournal(spec.EventsPath)
	if identityErr != nil {
		logFile.Close()
		return nil, identityErr
	}
	if closeErr := identityFile.Close(); closeErr != nil {
		logFile.Close()
		return nil, closeErr
	}
	spec.SourceIdentity = sourceIdentity
	if spec.ConversationJSONL {
		marker := ConversationSourceMarker{Version: 1, ProjectID: spec.ProjectID, WorkflowID: spec.WorkflowID, ExecutionID: spec.ExecutionID, AttemptID: spec.AttemptID, Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalProcess, Agent: spec.Agent, TargetID: spec.TargetID, Source: "codex-jsonl", SourceSegment: "local-jsonl-v1:" + sourceIdentity, SourceIdentity: sourceIdentity, RecordName: "events.jsonl", CreatedAt: now()}
		marker.ReaderCommand, marker.CodexHome = spec.ConversationReader, codexEnvironmentHome()
		if home, ok := spec.Env["CODEX_HOME"]; ok {
			marker.CodexHome = home
		}
		if err := marker.validate(); err != nil {
			logFile.Close()
			return nil, err
		}
		markerBytes, err := json.Marshal(marker)
		if err != nil {
			logFile.Close()
			return nil, err
		}
		if err = AtomicWrite(filepath.Join(filepath.Dir(spec.EventsPath), ConversationSourceMarkerName), append(markerBytes, '\n')); err != nil {
			logFile.Close()
			return nil, fmt.Errorf("persist conversation source marker: %w", err)
		}
	}
	errFile, e := os.OpenFile(spec.StderrPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		logFile.Close()
		return nil, e
	}
	var outputFile *os.File
	if spec.OutputPath != "" {
		outputFile, e = os.OpenFile(spec.OutputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			logFile.Close()
			errFile.Close()
			return nil, e
		}
	}
	cmd := exec.Command(spec.Command.Executable, spec.Command.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = unsetEnv(overrideEnv(cmd.Environ(), spec.Env), spec.UnsetEnv)
	cmd.Stdin = spec.Stdin
	cmd.Stderr = errFile
	// An explicit pipe lets the event reader finish before Wait, avoiding StdoutPipe races.
	reader, writer, e := os.Pipe()
	if e != nil {
		logFile.Close()
		errFile.Close()
		if outputFile != nil {
			outputFile.Close()
		}
		return nil, e
	}
	cmd.Stdout = writer
	p, e := startProcess(cmd)
	writer.Close()
	if e != nil {
		reader.Close()
		logFile.Close()
		errFile.Close()
		if outputFile != nil {
			outputFile.Close()
		}
		return nil, e
	}
	observeCtx, cancelObservation := context.WithCancel(context.Background())
	w := &Worker{proc: p, Events: make(chan WorkerEvent, 64), Done: make(chan error, 1), cancelObservation: cancelObservation, observationDone: make(chan struct{})}
	parsed := make(chan WorkerEvent, 128)
	go func() {
		defer close(w.observationDone)
		defer close(w.Events)
		for {
			select {
			case <-observeCtx.Done():
				return
			case ev, ok := <-parsed:
				if !ok {
					return
				}
				// Apply bounded backpressure so a busy worker cannot silently lose
				// conversation events before their persisted cursor advances.
				select {
				case w.Events <- ev:
				case <-observeCtx.Done():
					return
				}
			}
		}
	}()
	go func() {
		var wg sync.WaitGroup
		var scanErr error
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(parsed)
			defer reader.Close()
			writer := io.Writer(logFile)
			if outputFile != nil {
				writer = io.MultiWriter(logFile, &maxOutputWriter{writer: outputFile, remaining: maxWorkerOutputBytes})
			}
			scanErr = streamWorkerEventsWithEmit(io.TeeReader(reader, writer), NewCodexNormalizer(), func(ev WorkerEvent) {
				ev.RunID, ev.AttemptID = spec.RunID, spec.AttemptID
				ev.SourceIdentity = spec.SourceIdentity
				if spec.ConversationJSONL {
					ev.Source, ev.SourceSegment = "codex-jsonl", "local-jsonl-v1:"+spec.SourceIdentity
				} else {
					ev.Source, ev.SourceSegment = "local-command-output", "local-command-v1:"+spec.SourceIdentity
				}
				select {
				case parsed <- ev:
				case <-observeCtx.Done():
				}
			})
			if scanErr != nil {
				p.Kill()
			}
		}()
		waited := make(chan error, 1)
		go func() { waited <- cmd.Wait() }()
		var err error
		if spec.Timeout > 0 {
			timer := time.NewTimer(spec.Timeout)
			select {
			case err = <-waited:
			case <-timer.C:
				p.Kill()
				err = <-waited
				if err == nil {
					err = fmt.Errorf("worker step timed out after %s", spec.Timeout)
				}
			}
			timer.Stop()
		} else {
			err = <-waited
		}
		p.Close()
		wg.Wait()
		<-w.observationDone
		if scanErr != nil {
			err = scanErr
		}
		if e := logFile.Sync(); err == nil {
			err = e
		}
		if e := logFile.Close(); err == nil {
			err = e
		}
		if e := errFile.Close(); err == nil {
			err = e
		}
		if outputFile != nil {
			if e := outputFile.Sync(); err == nil {
				err = e
			}
			if e := outputFile.Close(); err == nil {
				err = e
			}
		}
		w.Done <- err
	}()
	return w, nil
}

// streamWorkerEvents parses one worker event stream, normalizes each line and
// emits one WorkerEvent per activity record. It is shared by every host.
func streamWorkerEvents(reader io.Reader, normalizer Normalizer, events chan<- WorkerEvent) error {
	return streamWorkerEventsWithEmit(reader, normalizer, func(ev WorkerEvent) { events <- ev })
}

func streamWorkerEventsWithEmit(reader io.Reader, normalizer Normalizer, emit func(WorkerEvent)) error {
	const maxEventLine = JournalMaxRecord + 1
	r := bufio.NewReaderSize(reader, 64<<10)
	line := make([]byte, 0, 64<<10)
	tooLong := false
	var sourceOffset, lineOffset int64
	var sourceSeq uint64
	for {
		fragment, err := r.ReadSlice('\n')
		if len(line) == 0 && !tooLong {
			lineOffset = sourceOffset
		}
		sourceOffset += int64(len(fragment))
		if !tooLong {
			if len(line)+len(fragment) > maxEventLine {
				line = line[:0]
				tooLong = true
			} else {
				line = append(line, fragment...)
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if !tooLong && len(line) > 0 {
			rawRecord := append([]byte(nil), line...)
			content := bytes.TrimSuffix(line, []byte{'\n'})
			if len(content) > 0 && content[len(content)-1] == '\r' {
				content = content[:len(content)-1]
			}
			sourceSeq++
			emitWorkerLine(content, rawRecord, normalizer, sourceSeq, lineOffset, int64(len(rawRecord)), emit)
		} else if tooLong && (err == nil || errors.Is(err, io.EOF)) {
			sourceSeq++
			emit(WorkerEvent{Summary: "oversized raw event", Activity: ActivityEvent{Source: normalizer.Name(), Kind: "unknown", Title: "oversized raw event", Meta: map[string]any{"source_bytes": sourceOffset - lineOffset}}, SourceGap: true, Diagnostic: fmt.Sprintf("source record exceeds %d bytes", JournalMaxRecord), SourceSeq: sourceSeq, SourceOffset: lineOffset, SourceLength: sourceOffset - lineOffset})
		}
		line = line[:0]
		tooLong = false
		if errors.Is(err, io.EOF) {
			return nil
		}
	}
}

func emitWorkerLine(line, rawRecord []byte, normalizer Normalizer, sourceSeq uint64, sourceOffset, sourceLength int64, emit func(WorkerEvent)) {
	event, err := codexworker.DecodeExecLine(line)
	if err != nil {
		emit(WorkerEvent{Summary: "unparsed raw event", Activity: ActivityEvent{Source: normalizer.Name(), Kind: "unknown", Title: "unparsed raw event", Meta: map[string]any{"raw_sha256": hashJournalBytes(line), "raw_bytes": len(line)}}, Raw: append([]byte(nil), rawRecord...), SourceSeq: sourceSeq, SourceOffset: sourceOffset, SourceLength: sourceLength, SourceSegment: "local-jsonl-v1"})
		return
	}
	summary, session := event.Summary, event.SessionID
	activities := normalizer.Normalize(line)
	if len(activities) == 0 {
		activities = []ActivityEvent{{Source: normalizer.Name(), Kind: "unknown", Title: event.Type, Raw: cloneRaw(line)}}
	}
	for _, activity := range activities {
		if len(activity.Raw) > activityDetailMax {
			activity.Raw = nil
			if activity.Meta == nil {
				activity.Meta = map[string]any{}
			}
			activity.Meta["raw_sha256"], activity.Meta["raw_bytes"] = hashJournalBytes(line), len(line)
		}
		emit(WorkerEvent{Session: session, Summary: summary, Activity: activity, Raw: append([]byte(nil), rawRecord...), SourceSeq: sourceSeq, SourceOffset: sourceOffset, SourceLength: sourceLength, SourceSegment: "local-jsonl-v1"})
	}
}
