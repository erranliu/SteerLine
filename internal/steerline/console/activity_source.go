package console

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/erranliu/SteerLine/internal/steerline"
)

type LogBatch struct {
	Cursor      steerline.JournalCursor
	Reset       bool
	Activity    []steerline.ActivityEvent
	Diagnostics []string
}

// fileSource reads the activity journal.
type fileSource struct {
	cfg              steerline.Config
	selectNormalizer func([]byte) steerline.Normalizer

	mu          sync.Mutex
	normalizers map[string]steerline.Normalizer
}

func newFileSource(c steerline.Config, selectors ...func([]byte) steerline.Normalizer) *fileSource {
	var selector func([]byte) steerline.Normalizer
	if len(selectors) > 0 {
		selector = selectors[0]
	}
	return &fileSource{cfg: c, selectNormalizer: selector, normalizers: make(map[string]steerline.Normalizer)}
}

func (f *fileSource) ReadLog(logPath string, cursor steerline.JournalCursor) (LogBatch, error) {
	if strings.TrimSpace(logPath) == "" {
		return LogBatch{}, fmt.Errorf("empty run log path")
	}
	path := logPath
	activityFile := false
	candidate := steerline.ActivityPath(logPath)
	if _, err := os.Stat(candidate); err == nil {
		path, activityFile = candidate, true
	}
	framed, err := steerline.ReadJournalBatch(path, cursor, steerline.JournalOptions{
		FromStart: !cursor.Initialized,
		Activity:  activityFile,
	})
	if err != nil {
		return LogBatch{}, err
	}
	batch := LogBatch{Cursor: framed.Cursor, Reset: framed.Reset, Diagnostics: append([]string(nil), framed.Diagnostics...)}
	if activityFile {
		for _, line := range framed.Lines {
			ev, err := steerline.DecodeActivityLine(line)
			if err != nil {
				batch.Diagnostics = append(batch.Diagnostics, fmt.Sprintf("invalid activity JSON: %v", err))
				continue
			}
			batch.Activity = append(batch.Activity, ev)
		}
		return batch, nil
	}
	if f.selectNormalizer == nil {
		return LogBatch{}, errors.New("application raw log normalizer selector is required")
	}
	var firstLine []byte
	if len(framed.Lines) > 0 {
		firstLine = framed.Lines[0]
	}
	normalizer := f.normalizerFor(path, framed.Cursor, firstLine, framed.Reset)
	for _, line := range framed.Lines {
		if normalizer == nil {
			normalizer = f.detect(line)
			f.storeNormalizer(path, framed.Cursor, normalizer)
		}
		batch.Activity = append(batch.Activity, steerline.NormalizeFallbackLine(normalizer, line, &framed.Cursor)...)
	}
	batch.Cursor = framed.Cursor
	return batch, nil
}

func (f *fileSource) normalizerFor(path string, cursor steerline.JournalCursor, firstRecord []byte, reset bool) steerline.Normalizer {
	key := fmt.Sprintf("%s|%s|%d", path, cursor.FileID, cursor.Generation)
	f.mu.Lock()
	defer f.mu.Unlock()
	if reset {
		delete(f.normalizers, key)
	}
	if n := f.normalizers[key]; n != nil {
		return n
	}
	first, err := steerline.FirstJournalLine(path)
	if err == nil && len(first) == 0 {
		first = firstRecord
	}
	if err != nil && len(firstRecord) > 0 {
		first, err = firstRecord, nil
	}
	if err != nil {
		first = nil
	}
	if len(first) == 0 {
		return nil
	}
	n := f.detect(first)
	if len(f.normalizers) >= 64 {
		f.normalizers = make(map[string]steerline.Normalizer)
	}
	f.normalizers[key] = n
	return n
}

func (f *fileSource) detect(first []byte) steerline.Normalizer {
	if f.selectNormalizer != nil {
		return f.selectNormalizer(first)
	}
	return nil
}

func (f *fileSource) storeNormalizer(path string, cursor steerline.JournalCursor, normalizer steerline.Normalizer) {
	if normalizer == nil {
		return
	}
	key := fmt.Sprintf("%s|%s|%d", path, cursor.FileID, cursor.Generation)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.normalizers) >= 64 {
		f.normalizers = make(map[string]steerline.Normalizer)
	}
	f.normalizers[key] = normalizer
}
