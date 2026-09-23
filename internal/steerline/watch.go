package steerline

import (
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
	Out       io.Writer
}

type watchedRun struct {
	RunID    string
	TaskID   string
	Title    string
	Status   string
	Events   string
	Activity string
}

// Watch streams one run and one source path, selected once at invocation time.
// It never mutates service state and reads records through the same bounded
// journal cursor used by the viewer.
func Watch(ctx context.Context, c Config, runID string, o WatchOptions) error {
	r, err := resolveWatchedRun(c, runID)
	if err != nil {
		return err
	}
	out := o.Out
	if out == nil {
		out = os.Stdout
	}
	if !o.JSON && !o.Raw {
		fmt.Fprintf(os.Stderr, "steerline watch · run %s · %s %q · %s\n", r.RunID, r.TaskID, r.Title, r.Status)
	}
	path := r.Events
	if o.Raw {
		if path == "" {
			return errors.New("run has no raw event log")
		}
	} else if r.Activity != "" && fileExists(r.Activity) {
		path = r.Activity
	} else if path == "" {
		return errors.New("no activity or event log for this run")
	}

	var normalizer Normalizer
	cursor := JournalCursor{}
	firstSourceLine := func() []byte {
		line, e := FirstJournalLine(path)
		if e != nil {
			return nil
		}
		return line
	}
	for {
		if ctx.Err() != nil {
			return nil
		}
		batch, readErr := ReadJournalBatch(path, cursor, JournalOptions{
			FromStart: o.FromStart,
			Activity:  !o.Raw && path == r.Activity,
			KeepCR:    o.Raw,
		})
		if readErr != nil {
			if o.Follow && os.IsNotExist(readErr) {
				if e := sleepOrDone(ctx); e != nil {
					return nil
				}
				continue
			}
			return readErr
		}
		cursor = batch.Cursor
		for _, diagnostic := range batch.Diagnostics {
			fmt.Fprintln(os.Stderr, "steerline watch:", diagnostic)
		}
		if batch.Reset && !o.Raw && path != r.Activity {
			first := firstSourceLine()
			if len(first) > 0 {
				normalizer = DetectNormalizer(first)
			} else {
				normalizer = nil
			}
		}
		for _, line := range batch.Lines {
			if ctx.Err() != nil {
				return nil
			}
			if o.Raw {
				if _, err = out.Write(append(append([]byte(nil), line...), '\n')); err != nil {
					return err
				}
				continue
			}
			if path == r.Activity {
				var ev ActivityEvent
				ev, err = DecodeActivityLine(line)
				if err != nil {
					fmt.Fprintf(os.Stderr, "steerline watch: invalid activity JSON: %v\n", err)
					continue
				}
				if o.JSON {
					if _, err = out.Write(append(append([]byte(nil), line...), '\n')); err != nil {
						return err
					}
				} else if _, err = fmt.Fprintln(out, RenderActivity(ev, RenderOptions{Color: o.Color})); err != nil {
					return err
				}
				continue
			}
			if normalizer == nil {
				normalizer = DetectNormalizer(line)
			}
			for _, ev := range NormalizeFallbackLine(normalizer, line, &cursor) {
				if o.JSON {
					var b []byte
					b, err = json.Marshal(ev)
					if err == nil {
						_, err = out.Write(append(b, '\n'))
					}
				} else {
					_, err = fmt.Fprintln(out, RenderActivity(ev, RenderOptions{Color: o.Color}))
				}
				if err != nil {
					return err
				}
			}
		}
		if !o.Follow && len(batch.Lines) == 0 && !batch.More {
			return nil
		}
		if len(batch.Lines) == 0 && o.Follow {
			if e := sleepOrDone(ctx); e != nil {
				return nil
			}
		}
	}
}

func resolveWatchedRun(c Config, runID string) (watchedRun, error) {
	snapshot, err := LoadRuntime(filepath.Join(c.RunDir, c.RuntimeFile))
	if err != nil {
		return watchedRun{}, err
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
		out.Activity = ActivityPath(*chosen.LogPath)
	}
	return out, nil
}

func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }

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
