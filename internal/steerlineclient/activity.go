package steerlineclient

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
)

type LogBatch = clientcontracts.LogBatch

// LogReader reads the activity journal.
type LogReader struct {
	selectNormalizer func([]byte) clientcontracts.LogNormalizer

	mu          sync.Mutex
	normalizers map[string]clientcontracts.LogNormalizer
}

// NewLogReader selects raw normalization supplied by application composition.
func NewLogReader(selector func([]byte) clientcontracts.LogNormalizer) *LogReader {
	return &LogReader{selectNormalizer: selector, normalizers: make(map[string]clientcontracts.LogNormalizer)}
}

func (f *LogReader) ReadLog(logPath string, cursor clientcontracts.JournalCursor) (LogBatch, error) {
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

func (f *LogReader) normalizerFor(path string, cursor clientcontracts.JournalCursor, firstRecord []byte, reset bool) clientcontracts.LogNormalizer {
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
		f.normalizers = make(map[string]clientcontracts.LogNormalizer)
	}
	f.normalizers[key] = n
	return n
}

func (f *LogReader) detect(first []byte) clientcontracts.LogNormalizer {
	if f.selectNormalizer != nil {
		return f.selectNormalizer(first)
	}
	return nil
}

func (f *LogReader) storeNormalizer(path string, cursor clientcontracts.JournalCursor, normalizer clientcontracts.LogNormalizer) {
	if normalizer == nil {
		return
	}
	key := fmt.Sprintf("%s|%s|%d", path, cursor.FileID, cursor.Generation)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.normalizers) >= 64 {
		f.normalizers = make(map[string]clientcontracts.LogNormalizer)
	}
	f.normalizers[key] = normalizer
}
