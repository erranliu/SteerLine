package steerline

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
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
	Dir        string
	EventsPath string
	StderrPath string
	OutputPath string
	Command    Command
	Stdin      io.Reader
	Env        map[string]string
	Timeout    time.Duration
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

type maxOutputWriter struct {
	writer    io.Writer
	remaining int64
}

func (w *maxOutputWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > w.remaining {
		return 0, fmt.Errorf("minimal step output exceeds %d bytes", minimalStepOutputMax)
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
	cmd.Env = overrideEnv(cmd.Environ(), spec.Env)
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
	w := &Worker{proc: p, Events: make(chan WorkerEvent, 64), Done: make(chan error, 1)}
	go func() {
		var wg sync.WaitGroup
		var scanErr error
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer reader.Close()
			writer := io.Writer(logFile)
			if outputFile != nil {
				writer = io.MultiWriter(logFile, &maxOutputWriter{writer: outputFile, remaining: minimalStepOutputMax})
			}
			scanErr = streamWorkerEvents(io.TeeReader(reader, writer), NewCodexNormalizer(), w.Events)
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
		close(w.Events)
		w.Done <- err
	}()
	return w, nil
}

// streamWorkerEvents parses one worker event stream, normalizes each line and
// emits one WorkerEvent per activity record. It is shared by every host.
func streamWorkerEvents(reader io.Reader, normalizer Normalizer, events chan<- WorkerEvent) error {
	s := bufio.NewScanner(reader)
	s.Buffer(make([]byte, 64<<10), 4<<20)
	for s.Scan() {
		line := s.Bytes()
		var event struct {
			Type     string `json:"type"`
			ThreadID string `json:"thread_id"`
			Item     struct {
				Type string `json:"type"`
			} `json:"item"`
		}
		if e := json.Unmarshal(line, &event); e != nil {
			continue
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
			events <- WorkerEvent{Session: session, Summary: summary, Activity: activity}
		}
	}
	return s.Err()
}
