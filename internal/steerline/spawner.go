package steerline

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
)

// process is the host-neutral handle for a running worker. Local uses an OS
// process; Orca uses a terminal handle. Neither call site needs to know which.
type process interface {
	Identity() ProcessIdentity
	Kill()
	Close()
}

// SpawnSpec is everything a host needs to run one worker. It contains no
// scheduling or recovery authority: the scheduler keeps ownership.
type SpawnSpec struct {
	RunID      string
	AttemptID  string
	Dir        string
	EventsPath string
	StderrPath string
	Command    Command
	Stdin      io.Reader
	Env        map[string]string
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

func (LocalSpawner) Host() string { return "local" }

func (LocalSpawner) Start(ctx context.Context, spec SpawnSpec) (*Worker, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	logFile, e := os.OpenFile(spec.EventsPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return nil, e
	}
	errFile, e := os.OpenFile(spec.StderrPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		logFile.Close()
		return nil, e
	}
	cmd := exec.Command(spec.Command.Executable, spec.Command.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = overrideEnv(cmd.Environ(), spec.Env)
	cmd.Stdin = spec.Stdin
	cmd.Stderr = errFile
	// An explicit pipe lets the event reader finish before Wait, avoiding StdoutPipe races.
	reader, writer, e := os.Pipe()
	if e != nil {
		logFile.Close()
		errFile.Close()
		return nil, e
	}
	cmd.Stdout = writer
	p, e := startProcess(cmd)
	writer.Close()
	if e != nil {
		reader.Close()
		logFile.Close()
		errFile.Close()
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
				// events.jsonl remains complete even when the observation queue is full.
				select {
				case w.Events <- ev:
				default:
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
			scanErr = streamWorkerEventsWithEmit(io.TeeReader(reader, logFile), NewCodexNormalizer(), func(ev WorkerEvent) {
				ev.RunID, ev.AttemptID = spec.RunID, spec.AttemptID
				select {
				case parsed <- ev:
				default:
				}
			})
			if scanErr != nil {
				p.Kill()
			}
		}()
		err := cmd.Wait()
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
	const maxEventLine = 4 << 20
	r := bufio.NewReaderSize(reader, 64<<10)
	line := make([]byte, 0, 64<<10)
	tooLong := false
	for {
		fragment, err := r.ReadSlice('\n')
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
			content := line
			if content[len(content)-1] == '\n' {
				content = content[:len(content)-1]
			}
			if len(content) > 0 && content[len(content)-1] == '\r' {
				content = content[:len(content)-1]
			}
			emitWorkerLine(content, normalizer, emit)
		}
		line = line[:0]
		tooLong = false
		if errors.Is(err, io.EOF) {
			return nil
		}
	}
}

func emitWorkerLine(line []byte, normalizer Normalizer, emit func(WorkerEvent)) {
	var event struct {
		Type     string `json:"type"`
		ThreadID string `json:"thread_id"`
		Item     struct {
			Type string `json:"type"`
		} `json:"item"`
	}
	if e := json.Unmarshal(line, &event); e != nil {
		return
	}
	summary := clip(event.Type+" "+event.Item.Type, 512)
	session := ""
	if event.Type == "thread.started" {
		session = event.ThreadID
	}
	activities := normalizer.Normalize(line)
	if len(activities) == 0 {
		activities = []ActivityEvent{{Source: normalizer.Name(), Kind: "unknown", Title: event.Type, Raw: cloneRaw(line)}}
	}
	for _, activity := range activities {
		emit(WorkerEvent{Session: session, Summary: summary, Activity: activity})
	}
}
