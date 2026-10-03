package codexworker_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"io"
)

type nativeStreamEvent struct {
	Summary, Session, Diagnostic string
	Activity                     contracts.ActivityEvent
	Raw                          []byte
	SourceGap                    bool
	SourceSeq                    uint64
	SourceOffset, SourceLength   int64
}
type nativeStreamNormalizer interface {
	Name() string
	Normalize([]byte) []contracts.ActivityEvent
}

const nativeStreamRecordLimit = 8 << 20
const nativeStreamActivityLimit = 4096

func nativeStreamHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func nativeStreamEvents(reader io.Reader, normalizer nativeStreamNormalizer, events chan<- nativeStreamEvent) error {
	return nativeStreamEventsWithEmit(reader, normalizer, func(ev nativeStreamEvent) { events <- ev })
}

func nativeStreamEventsWithEmit(reader io.Reader, normalizer nativeStreamNormalizer, emit func(nativeStreamEvent)) error {
	const maxEventLine = nativeStreamRecordLimit + 1
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
			emit(nativeStreamEvent{Summary: "oversized raw event", Activity: contracts.ActivityEvent{Source: normalizer.Name(), Kind: "unknown", Title: "oversized raw event", Meta: map[string]any{"source_bytes": sourceOffset - lineOffset}}, SourceGap: true, Diagnostic: fmt.Sprintf("source record exceeds %d bytes", nativeStreamRecordLimit), SourceSeq: sourceSeq, SourceOffset: lineOffset, SourceLength: sourceOffset - lineOffset})
		}
		line = line[:0]
		tooLong = false
		if errors.Is(err, io.EOF) {
			return nil
		}
	}
}

func emitWorkerLine(line, rawRecord []byte, normalizer nativeStreamNormalizer, sourceSeq uint64, sourceOffset, sourceLength int64, emit func(nativeStreamEvent)) {
	event, err := codexworker.DecodeExecLine(line)
	if err != nil {
		emit(nativeStreamEvent{Summary: "unparsed raw event", Activity: contracts.ActivityEvent{Source: normalizer.Name(), Kind: "unknown", Title: "unparsed raw event", Meta: map[string]any{"raw_sha256": nativeStreamHash(line), "raw_bytes": len(line)}}, Raw: append([]byte(nil), rawRecord...), SourceSeq: sourceSeq, SourceOffset: sourceOffset, SourceLength: sourceLength})
		return
	}
	summary, session := event.Summary, event.SessionID
	activities := normalizer.Normalize(line)
	if len(activities) == 0 {
		activities = []contracts.ActivityEvent{{Source: normalizer.Name(), Kind: "unknown", Title: event.Type, Raw: append([]byte(nil), line...)}}
	}
	for _, activity := range activities {
		if len(activity.Raw) > nativeStreamActivityLimit {
			activity.Raw = nil
			if activity.Meta == nil {
				activity.Meta = map[string]any{}
			}
			activity.Meta["raw_sha256"], activity.Meta["raw_bytes"] = nativeStreamHash(line), len(line)
		}
		emit(nativeStreamEvent{Session: session, Summary: summary, Activity: activity, Raw: append([]byte(nil), rawRecord...), SourceSeq: sourceSeq, SourceOffset: sourceOffset, SourceLength: sourceLength})
	}
}
