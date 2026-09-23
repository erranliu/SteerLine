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
	"path/filepath"
	"time"
)

// WatchOptions controls the stable `steerline watch` entry point.
type WatchOptions struct {
	Follow    bool
	FromStart bool
	JSON      bool
	Raw       bool
	Color     bool
	// Out receives rendered or JSON lines. Nil means os.Stdout.
	Out io.Writer
}

type watchedRun struct {
	RunID    string
	TaskID   string
	Title    string
	Status   string
	Events   string
	Activity string
}

// Watch streams a run's activity. It never mutates service state and never
// requires the service to be running: it reads runtime.json and the per-run
// files only. The JSONL contract it emits is documented in docs/activity.md.
func Watch(ctx context.Context, c Config, runID string, o WatchOptions) error {
	r, e := resolveWatchedRun(c, runID)
	if e != nil {
		return e
	}
	out := o.Out
	if out == nil {
		out = os.Stdout
	}
	if !o.JSON {
		fmt.Fprintf(os.Stderr, "steerline watch · run %s · %s %q · %s\n", r.RunID, r.TaskID, r.Title, r.Status)
	}
	if o.Raw {
		if r.Events == "" {
			return errors.New("run has no raw event log")
		}
		return followLines(ctx, r.Events, o.Follow, o.FromStart, func(line []byte, _ bool) error {
			_, e := out.Write(append(append([]byte{}, line...), '\n'))
			return e
		})
	}
	// Prefer the normalized activity log. Fall back to normalizing the raw agent
	// stream so watch still works for runs produced by an older binary or by a
	// raw host capture.
	if r.Activity != "" && fileExists(r.Activity) {
		return followLines(ctx, r.Activity, o.Follow, o.FromStart, func(line []byte, _ bool) error {
			var ev ActivityEvent
			if json.Unmarshal(line, &ev) != nil {
				return nil
			}
			if o.JSON {
				_, e := out.Write(append(append([]byte{}, line...), '\n'))
				return e
			}
			_, e := fmt.Fprintln(out, RenderActivity(ev, RenderOptions{Color: o.Color}))
			return e
		})
	}
	if r.Events == "" {
		return errors.New("no activity or event log for this run")
	}
	var normalizer Normalizer
	return followLines(ctx, r.Events, o.Follow, o.FromStart, func(line []byte, first bool) error {
		if normalizer == nil {
			if first {
				normalizer = DetectNormalizer(line)
			} else {
				normalizer = NewCodexNormalizer()
			}
		}
		events := normalizer.Normalize(line)
		for _, ev := range events {
			if o.JSON {
				b, e := json.Marshal(ev)
				if e != nil {
					continue
				}
				if _, e = out.Write(append(b, '\n')); e != nil {
					return e
				}
				continue
			}
			if _, e := fmt.Fprintln(out, RenderActivity(ev, RenderOptions{Color: o.Color})); e != nil {
				return e
			}
		}
		return nil
	})
}

func resolveWatchedRun(c Config, runID string) (watchedRun, error) {
	snapshot, e := LoadRuntime(filepath.Join(c.RunDir, c.RuntimeFile))
	if e != nil {
		return watchedRun{}, e
	}
	if snapshot == nil {
		return watchedRun{}, errors.New("no runtime snapshot; start the service or pass an existing run")
	}
	var chosen *Run
	if runID != "" {
		for _, r := range snapshot.Runs {
			if r != nil && (r.RunID == runID || r.TaskID == runID) {
				chosen = r
				break
			}
		}
		if chosen == nil {
			return watchedRun{}, fmt.Errorf("run %q not found in runtime", runID)
		}
	} else {
		for _, r := range snapshot.Runs {
			if r != nil && r.ExecutionStatus == "running" {
				chosen = r
			}
		}
		if chosen == nil && len(snapshot.Runs) > 0 {
			chosen = snapshot.Runs[len(snapshot.Runs)-1]
		}
	}
	if chosen == nil {
		return watchedRun{}, errors.New("no runs recorded")
	}
	out := watchedRun{RunID: chosen.RunID, TaskID: chosen.TaskID, Title: chosen.Title, Status: chosen.ExecutionStatus + "/" + chosen.FinishTaskStatus}
	if chosen.LogPath != nil {
		out.Events = *chosen.LogPath
		out.Activity = activityPath(*chosen.LogPath)
	}
	return out, nil
}

func fileExists(path string) bool {
	_, e := os.Stat(path)
	return e == nil
}

// followLines reads newline-delimited records, tolerating a partial trailing
// line, file truncation and atomic replacement. handle receives the line without
// the newline and whether it is the first line observed.
func followLines(ctx context.Context, path string, follow, fromStart bool, handle func(line []byte, first bool) error) error {
	if path == "" {
		return errors.New("empty log path")
	}
	var f *os.File
	var reader *bufio.Reader
	var offset int64
	var pending []byte
	first := true
	defer func() {
		if f != nil {
			f.Close()
		}
	}()
	for {
		if f == nil {
			opened, e := os.Open(path)
			if e != nil {
				if !follow || !os.IsNotExist(e) {
					return e
				}
				if e := sleepOrDone(ctx); e != nil {
					return nil
				}
				continue
			}
			f = opened
			reader = bufio.NewReaderSize(f, 64<<10)
			offset = 0
			if !fromStart {
				if end, se := f.Seek(0, io.SeekEnd); se == nil {
					reader.Reset(f)
					offset = end
				}
			}
		}
		chunk := make([]byte, 64<<10)
		n, e := reader.Read(chunk)
		if n > 0 {
			offset += int64(n)
			pending = append(pending, chunk[:n]...)
			for {
				i := bytes.IndexByte(pending, '\n')
				if i < 0 {
					break
				}
				line := pending[:i]
				if err := handle(line, first); err != nil {
					return err
				}
				first = false
				pending = pending[i+1:]
			}
		}
		if e != nil && e != io.EOF {
			return e
		}
		if st, se := os.Stat(path); se == nil && st.Size() < offset {
			f.Close()
			f = nil
			pending = nil
			continue
		}
		if !follow {
			if e == io.EOF {
				return nil
			}
			continue
		}
		if e := sleepOrDone(ctx); e != nil {
			return nil
		}
	}
}

func sleepOrDone(ctx context.Context) error {
	t := time.NewTimer(200 * time.Millisecond)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
