package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/erranliu/SteerLine/internal/steerline/persistence"
)

// conversationJournalPosition is the committed source position. The two short
// anchors detect a same-file rewrite before any saved position is reused.
type conversationJournalPosition struct {
	Version      int    `json:"v"`
	ByteOffset   int64  `json:"byte_offset"`
	SourceSeq    uint64 `json:"source_seq"`
	Generation   uint64 `json:"generation"`
	FileID       string `json:"file_id"`
	FileSize     int64  `json:"file_size"`
	ModifiedNS   int64  `json:"modified_ns"`
	HeaderLength int    `json:"header_length,omitempty"`
	HeaderHash   string `json:"header_hash,omitempty"`
	AnchorOffset int64  `json:"anchor_offset,omitempty"`
	AnchorLength int    `json:"anchor_length,omitempty"`
	AnchorHash   string `json:"anchor_hash,omitempty"`
}

func decodeConversationJournalPosition(checkpoint ConversationCheckpoint) (conversationJournalPosition, error) {
	var position conversationJournalPosition
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &position); err != nil {
		return position, err
	}
	if position.Version != 2 || position.ByteOffset < 0 || position.FileID == "" || position.FileID != checkpoint.SourceIdentity || position.Generation == 0 || position.SourceSeq != checkpoint.LocalSeq || position.HeaderLength < 0 || position.HeaderLength > journalAnchorSize || position.AnchorLength < 0 || position.AnchorLength > journalAnchorSize || position.AnchorOffset < 0 || position.FileSize < position.ByteOffset || position.ModifiedNS == 0 {
		return position, errors.New("unsupported or invalid conversation source checkpoint; preserve source and database for manual recovery")
	}
	if position.ByteOffset > 0 && (position.AnchorLength == 0 || position.AnchorHash == "" || position.HeaderLength == 0 || position.HeaderHash == "") {
		return position, errors.New("conversation checkpoint lacks durable source anchors")
	}
	return position, nil
}

func (p conversationJournalPosition) journalCursor(path string) JournalCursor {
	return JournalCursor{Path: path, FileID: p.FileID, Generation: p.Generation, Offset: p.ByteOffset, ScanOffset: p.ByteOffset, HeaderLength: p.HeaderLength, HeaderHash: p.HeaderHash, AnchorOffset: p.AnchorOffset, AnchorLength: p.AnchorLength, AnchorHash: p.AnchorHash, Initialized: true}
}

func currentConversationJournalPosition(ctx context.Context, store persistence.Conversations, project, execution, attempt, source, segment string, generation uint64, path, fileID string, offset int64, seq uint64) (conversationJournalPosition, error) {
	if generation == 0 {
		return conversationJournalPosition{}, errors.New("conversation source generation is invalid")
	}
	var previous *conversationJournalPosition
	checkpoint, err := store.ConversationCheckpoint(ctx, project, execution, attempt, source, segment)
	if err == nil {
		position, decodeErr := decodeConversationJournalPosition(checkpoint)
		if decodeErr != nil {
			return conversationJournalPosition{}, decodeErr
		}
		previous = &position
	} else if !errors.Is(err, persistence.ErrNotFound) {
		return conversationJournalPosition{}, err
	}
	return readConversationJournalPosition(path, fileID, offset, seq, generation, previous)
}

func readConversationJournalPosition(path, fileID string, offset int64, seq, generation uint64, previous *conversationJournalPosition) (conversationJournalPosition, error) {
	position := conversationJournalPosition{Version: 2, ByteOffset: offset, SourceSeq: seq, Generation: generation, FileID: fileID}
	if offset < 0 || fileID == "" || generation == 0 {
		return position, errors.New("invalid conversation source position")
	}
	f, id, err := openJournal(path)
	if err != nil {
		return position, err
	}
	defer f.Close()
	if id != fileID {
		return position, errors.New("conversation file identity changed before checkpoint")
	}
	stat, err := f.Stat()
	if err != nil {
		return position, err
	}
	if offset > stat.Size() {
		return position, errors.New("conversation source shrank before checkpoint")
	}
	position.FileSize, position.ModifiedNS = stat.Size(), stat.ModTime().UnixNano()
	if previous != nil {
		if previous.FileID != fileID || previous.Generation != generation || previous.ByteOffset > offset {
			return position, errors.New("conversation checkpoint cannot change identity or move backwards")
		}
		if err = checkConversationPositionAnchors(f, *previous); err != nil {
			return position, err
		}
		if stat.Size() == previous.FileSize && position.ModifiedNS != previous.ModifiedNS && previous.ByteOffset > 0 {
			return position, errors.New("conversation source changed without an append before checkpoint")
		}
		position.HeaderLength, position.HeaderHash = previous.HeaderLength, previous.HeaderHash
	} else if stat.Size() > 0 {
		position.HeaderLength = journalAnchorSize
		if stat.Size() < int64(position.HeaderLength) {
			position.HeaderLength = int(stat.Size())
		}
		b := make([]byte, position.HeaderLength)
		if _, err = f.ReadAt(b, 0); err != nil {
			return position, err
		}
		position.HeaderHash = hashJournalBytes(b)
	}
	if offset > 0 {
		position.AnchorLength = journalAnchorSize
		if offset < int64(position.AnchorLength) {
			position.AnchorLength = int(offset)
		}
		position.AnchorOffset = offset - int64(position.AnchorLength)
		b := make([]byte, position.AnchorLength)
		if _, err = f.ReadAt(b, position.AnchorOffset); err != nil {
			return position, err
		}
		position.AnchorHash = hashJournalBytes(b)
	}
	return position, nil
}

func checkConversationPositionAnchors(f *os.File, p conversationJournalPosition) error {
	check := func(offset int64, length int, expected string) error {
		if length == 0 {
			return nil
		}
		b := make([]byte, length)
		if _, err := f.ReadAt(b, offset); err != nil && err != io.EOF {
			return err
		}
		if hashJournalBytes(b) != expected {
			return fmt.Errorf("conversation source changed at saved offset %d", offset)
		}
		return nil
	}
	if err := check(0, p.HeaderLength, p.HeaderHash); err != nil {
		return err
	}
	return check(p.AnchorOffset, p.AnchorLength, p.AnchorHash)
}

func marshalConversationJournalPosition(p conversationJournalPosition) (string, error) {
	b, err := json.Marshal(p)
	return string(b), err
}
