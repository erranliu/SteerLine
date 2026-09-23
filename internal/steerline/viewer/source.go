package viewer

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/erranliu/SteerLine/internal/steerline"
)

// Source is the read-mostly data the viewer needs. Only Answer writes, and it
// writes only the immutable decision answer file; runtime.json is never touched
// from here. A fake implementation makes the UI fully testable.
type Source interface {
	Runs() ([]*steerline.Run, error)
	Activity(logPath string, from int64) ([]steerline.ActivityEvent, int64, error)
	Answer(run *steerline.Run, optionID, note, by string) error
}

// FileSource reads the same files `steerline watch` reads.
type FileSource struct{ cfg steerline.Config }

func NewFileSource(c steerline.Config) *FileSource { return &FileSource{cfg: c} }

func (f *FileSource) Runs() ([]*steerline.Run, error) {
	rt, e := steerline.LoadRuntime(filepath.Join(f.cfg.RunDir, f.cfg.RuntimeFile))
	if e != nil {
		return nil, e
	}
	if rt == nil {
		return nil, nil
	}
	return rt.Runs, nil
}

// Activity reads the run's normalized journal from a byte offset and returns the
// events plus the next offset, so the viewer only ever reads new lines.
func (f *FileSource) Activity(logPath string, from int64) ([]steerline.ActivityEvent, int64, error) {
	file, e := os.Open(steerline.ActivityPath(logPath))
	if e != nil {
		return nil, from, e
	}
	defer file.Close()
	if from > 0 {
		if _, e := file.Seek(from, io.SeekStart); e != nil {
			return nil, from, e
		}
	}
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var events []steerline.ActivityEvent
	off := from
	for sc.Scan() {
		line := sc.Bytes()
		off += int64(len(line)) + 1
		var ev steerline.ActivityEvent
		if json.Unmarshal(line, &ev) == nil {
			events = append(events, ev)
		}
	}
	return events, off, sc.Err()
}

func (f *FileSource) Answer(run *steerline.Run, optionID, note, by string) error {
	if by == "" {
		by = steerline.AnswererName()
	}
	return steerline.WriteAnswer(run, steerline.DecisionAnswer{OptionID: optionID, Note: note, AnsweredBy: by})
}
