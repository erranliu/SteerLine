package runnercontract

import "time"

const ProgressContract = "steerrunner-progress-file/v1"
const MaxProgressBytes = 64 << 10

type ProgressFileInput struct {
	InitialContentBase64 string `json:"initial_content_base64"`
}

// ProgressFile is the public opaque snapshot; business interpretation belongs to SteerLine.
type ProgressFile struct {
	Contract         string     `json:"contract"`
	RunID            string     `json:"run_id"`
	Version          uint64     `json:"version"`
	ObservedAt       *time.Time `json:"observed_at,omitempty"`
	Encoding         string     `json:"encoding"`
	ContentBase64    *string    `json:"content_base64,omitempty"`
	SizeBytes        int        `json:"size_bytes"`
	SHA256           string     `json:"sha256,omitempty"`
	CollectionStatus string     `json:"collection_status"`
	Diagnostic       string     `json:"diagnostic,omitempty"`
}
