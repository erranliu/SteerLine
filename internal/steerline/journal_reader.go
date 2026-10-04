package steerline

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

const (
	JournalMaxRecords = 256
	JournalMaxBytes   = 1 << 20
	JournalMaxRecord  = 8 << 20
	journalBufferSize = 64 << 10
	journalAnchorSize = 64
)

type JournalOptions struct {
	FromStart  bool
	KeepCR     bool // Raw exports preserve CRLF bytes; normalized consumers omit CR.
	MaxRecords int
	MaxBytes   int
}

type JournalBatch struct {
	Lines       [][]byte
	Records     []JournalRecord
	Cursor      productapi.JournalCursor
	Reset       bool
	More        bool
	Diagnostics []string
}

type JournalRecord struct {
	Offset    int64
	Length    int64
	Data      []byte
	Oversized bool
}

// ReadJournalBatch reads at most one bounded batch of complete UTF-8/byte
// records. It is deliberately representation-agnostic; consumers normalize or
// validate lines after this shared framing layer.
func ReadJournalBatch(path string, cursor productapi.JournalCursor, opts JournalOptions) (JournalBatch, error) {
	if strings.TrimSpace(path) == "" {
		return JournalBatch{}, errors.New("empty journal path")
	}
	if opts.MaxRecords <= 0 || opts.MaxRecords > JournalMaxRecords {
		opts.MaxRecords = JournalMaxRecords
	}
	if opts.MaxBytes <= 0 || opts.MaxBytes > JournalMaxBytes {
		opts.MaxBytes = JournalMaxBytes
	}

	f, id, err := openJournal(path)
	if err != nil {
		return JournalBatch{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return JournalBatch{}, err
	}

	batch := JournalBatch{Cursor: cursor}
	firstRead := !cursor.Initialized
	reset := !cursor.Initialized || cursor.Path != path
	if cursor.Initialized && cursor.Path == path {
		switch {
		case cursor.FileID != id:
			reset = true
		case info.Size() < cursor.ScanOffset || info.Size() < cursor.Offset:
			reset = true
		case !journalAnchorsMatch(f, cursor):
			reset = true
		}
	}
	if reset {
		generation := cursor.Generation + 1
		if generation == 0 {
			generation = 1
		}
		cursor = productapi.JournalCursor{Path: path, FileID: id, Generation: generation, Initialized: true}
		batch.Reset = true
	}
	if !cursor.Initialized {
		cursor.Initialized = true
	}
	cursor.Path, cursor.FileID = path, id
	if cursor.Generation == 0 {
		cursor.Generation = 1
	}
	if firstRead && !opts.FromStart {
		cursor.Offset = info.Size()
		cursor.ScanOffset = info.Size()
		if info.Size() > 0 {
			var last [1]byte
			if _, err := f.ReadAt(last[:], info.Size()-1); err == nil && last[0] != '\n' {
				cursor.SkipUntilNewline = true
			}
		}
		captureJournalAnchors(f, &cursor, info.Size())
		batch.Cursor = cursor
		return batch, nil
	}
	if _, err := f.Seek(cursor.ScanOffset, io.SeekStart); err != nil {
		return JournalBatch{}, err
	}
	// Bound physical work even when a record is huge and never terminates.
	// The extra record allowance permits one legal large line to own a batch.
	readBudget := int64(JournalMaxRecord + opts.MaxBytes + 1)
	reader := bufio.NewReaderSize(io.LimitReader(f, readBudget), journalBufferSize)
	line := append([]byte(nil), cursor.Pending...)
	pendingLength := len(cursor.Pending)
	cursor.Pending = nil
	batchBytes := 0
	processedRecords := 0
	for processedRecords < opts.MaxRecords {
		if len(batch.Lines) > 0 && batchBytes >= opts.MaxBytes {
			break
		}
		lineStart := cursor.ScanOffset
		if processedRecords == 0 && pendingLength > 0 {
			lineStart -= int64(pendingLength)
		}
		recordStart := lineStart
		if cursor.Dropping {
			recordStart = cursor.DroppingStart
		}
		completed, tooLong, scanned, lineBytes, readErr := readJournalLine(reader, line, cursor.ScanOffset)
		cursor.ScanOffset = scanned
		line = nil
		if !completed {
			if cursor.Dropping || tooLong {
				// Keep only a flag while waiting for a terminator; the oversized
				// prefix is never retained in memory.
				line = nil
				if !cursor.Dropping {
					cursor.DroppingStart = recordStart
				}
				cursor.Dropping = true
			} else {
				cursor.Pending = lineBytes
			}
			if readErr != nil && readErr != io.EOF {
				return JournalBatch{}, readErr
			}
			break
		}
		end := cursor.ScanOffset
		processedRecords++
		if cursor.SkipUntilNewline {
			cursor.SkipUntilNewline = false
			cursor.Offset = end
			continue
		}
		if cursor.Dropping || tooLong {
			batch.Records = append(batch.Records, JournalRecord{Offset: recordStart, Length: end - recordStart, Oversized: true})
			cursor.Dropping = false
			cursor.DroppingStart = 0
			cursor.Offset = end
			batch.Diagnostics = append(batch.Diagnostics, fmt.Sprintf("skipped journal record exceeding %d bytes", JournalMaxRecord))
			continue
		}
		rawLine := lineBytes
		if !opts.KeepCR {
			lineBytes = bytes.TrimSuffix(lineBytes, []byte{'\r'})
		}
		if len(batch.Lines) > 0 && batchBytes+len(lineBytes) > opts.MaxBytes {
			// Leave this complete line for the next call, including oversized
			// legal records that must not share a batch with preceding records.
			cursor.ScanOffset = lineStart
			break
		}
		batch.Lines = append(batch.Lines, append([]byte(nil), lineBytes...))
		batch.Records = append(batch.Records, JournalRecord{Offset: recordStart, Length: end - recordStart, Data: append([]byte(nil), rawLine...)})
		batchBytes += len(lineBytes)
		cursor.Offset = end
		captureLineAnchor(&cursor, rawLine, end-int64(len(rawLine))-1)
		line = nil
		if readErr != nil && readErr != io.EOF {
			return JournalBatch{}, readErr
		}
	}
	captureJournalAnchors(f, &cursor, info.Size())
	batch.More = cursor.ScanOffset < info.Size()
	batch.Cursor = cursor
	return batch, nil
}

func readJournalLine(reader *bufio.Reader, prefix []byte, scanned int64) (bool, bool, int64, []byte, error) {
	line := append([]byte(nil), prefix...)
	tooLong := len(line) > JournalMaxRecord
	for {
		part, err := reader.ReadSlice('\n')
		scanned += int64(len(part))
		if !tooLong {
			if len(line)+len(part) > JournalMaxRecord+1 {
				tooLong = true
				line = nil
			} else {
				line = append(line, part...)
			}
		}
		if err == nil {
			if tooLong {
				return true, true, scanned, nil, nil
			}
			if !tooLong && len(line) > 0 {
				line = line[:len(line)-1]
			}
			return true, false, scanned, line, nil
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if err == io.EOF {
			if tooLong {
				return false, true, scanned, nil, io.EOF
			}
			return false, false, scanned, line, io.EOF
		}
		return false, tooLong, scanned, line, err
	}
}

func journalAnchorsMatch(f *os.File, c productapi.JournalCursor) bool {
	if c.HeaderHash != "" && c.HeaderLength > 0 {
		n := c.HeaderLength
		b := make([]byte, n)
		got, err := f.ReadAt(b, 0)
		if err != nil && err != io.EOF {
			return false
		}
		if got != n || hashJournalBytes(b) != c.HeaderHash {
			return false
		}
	}
	if c.AnchorHash != "" && c.AnchorLength > 0 {
		b := make([]byte, c.AnchorLength)
		got, err := f.ReadAt(b, c.AnchorOffset)
		if err != nil && err != io.EOF {
			return false
		}
		if got != len(b) || hashJournalBytes(b) != c.AnchorHash {
			return false
		}
	}
	return true
}

func captureJournalAnchors(f *os.File, c *productapi.JournalCursor, size int64) {
	if c.HeaderHash == "" && size > 0 {
		n := journalAnchorSize
		if size < int64(n) {
			n = int(size)
		}
		b := make([]byte, n)
		if got, err := f.ReadAt(b, 0); err == nil || err == io.EOF && got == n {
			if got == n {
				c.HeaderLength = n
				c.HeaderHash = hashJournalBytes(b)
			}
		}
	}
}

func captureLineAnchor(c *productapi.JournalCursor, line []byte, offset int64) {
	c.AnchorOffset = offset
	c.AnchorLength = len(line)
	c.AnchorHash = hashJournalBytes(line)
}

func hashJournalBytes(b []byte) string { sum := sha256.Sum256(b); return fmt.Sprintf("%x", sum[:]) }
