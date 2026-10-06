package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	"io"
	"os"

	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// executionvalues.ConversationJournalPosition is the committed source position. The two short
// anchors detect a same-file rewrite before any saved position is reused.

func journalCursorOf(p executionvalues.ConversationJournalPosition, path string) JournalCursor {
	return JournalCursor{Path: path, FileID: p.FileID, Generation: p.Generation, Offset: p.ByteOffset, ScanOffset: p.ByteOffset, HeaderLength: p.HeaderLength, HeaderHash: p.HeaderHash, AnchorOffset: p.AnchorOffset, AnchorLength: p.AnchorLength, AnchorHash: p.AnchorHash, Initialized: true}
}

func currentConversationJournalPosition(ctx context.Context, store ConversationStore, project, execution, attempt, source, segment string, generation uint64, path, fileID string, offset int64, seq uint64) (executionvalues.ConversationJournalPosition, error) {
	if generation == 0 {
		return executionvalues.ConversationJournalPosition{}, errors.New("conversation source generation is invalid")
	}
	var previous *executionvalues.ConversationJournalPosition
	checkpoint, err := store.ConversationCheckpoint(ctx, project, execution, attempt, source, segment)
	if err == nil {
		position, decodeErr := executionvalues.DecodeConversationJournalPosition(checkpoint)
		if decodeErr != nil {
			return executionvalues.ConversationJournalPosition{}, decodeErr
		}
		previous = &position
	} else if !errors.Is(err, storageports.ErrNotFound) {
		return executionvalues.ConversationJournalPosition{}, err
	}
	return readConversationJournalPosition(path, fileID, offset, seq, generation, previous)
}

func readConversationJournalPosition(path, fileID string, offset int64, seq, generation uint64, previous *executionvalues.ConversationJournalPosition) (executionvalues.ConversationJournalPosition, error) {
	position := executionvalues.ConversationJournalPosition{Version: 2, ByteOffset: offset, SourceSeq: seq, Generation: generation, FileID: fileID}
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

func checkConversationPositionAnchors(f *os.File, p executionvalues.ConversationJournalPosition) error {
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

func marshalConversationJournalPosition(p executionvalues.ConversationJournalPosition) (string, error) {
	b, err := json.Marshal(p)
	return string(b), err
}
