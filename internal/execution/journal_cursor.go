package execution

// JournalCursor is execution-owned incremental journal scan state.
type JournalCursor struct {
	Path             string `json:"path,omitempty"`
	FileID           string `json:"file_id,omitempty"`
	Generation       uint64 `json:"generation,omitempty"`
	Offset           int64  `json:"offset,omitempty"`
	ScanOffset       int64  `json:"scan_offset,omitempty"`
	Pending          []byte `json:"pending,omitempty"`
	Dropping         bool   `json:"dropping,omitempty"`
	DroppingStart    int64  `json:"dropping_start,omitempty"`
	SkipUntilNewline bool   `json:"skip_until_newline,omitempty"`
	AnchorOffset     int64  `json:"anchor_offset,omitempty"`
	AnchorLength     int    `json:"anchor_length,omitempty"`
	AnchorHash       string `json:"anchor_hash,omitempty"`
	HeaderLength     int    `json:"header_length,omitempty"`
	HeaderHash       string `json:"header_hash,omitempty"`
	Initialized      bool   `json:"initialized,omitempty"`
}
