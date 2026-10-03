package contracts

import (
	"encoding/json"
	"errors"
)

type ConversationJournalPosition struct {
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

func DecodeConversationJournalPosition(checkpoint ConversationCheckpoint) (ConversationJournalPosition, error) {
	var position ConversationJournalPosition
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &position); err != nil {
		return position, err
	}
	if position.Version != 2 || position.ByteOffset < 0 || position.FileID == "" || position.FileID != checkpoint.SourceIdentity || position.Generation == 0 || position.SourceSeq != checkpoint.LocalSeq || position.HeaderLength < 0 || position.HeaderLength > 64 || position.AnchorLength < 0 || position.AnchorLength > 64 || position.AnchorOffset < 0 || position.FileSize < position.ByteOffset || position.ModifiedNS == 0 {
		return position, errors.New("unsupported or invalid conversation source checkpoint; preserve source and database for manual recovery")
	}
	if position.ByteOffset > 0 && (position.AnchorLength == 0 || position.AnchorHash == "" || position.HeaderLength == 0 || position.HeaderHash == "") {
		return position, errors.New("conversation checkpoint lacks durable source anchors")
	}
	return position, nil
}
