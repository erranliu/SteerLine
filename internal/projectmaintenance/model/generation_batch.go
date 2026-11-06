package model

import "time"

// A preparation freezes membership, source and profile. Generation records
// remain the result owner.
type GenerationBatch struct {
	ObservedAt         *time.Time          `json:"observed_at,omitempty"`
	CreatedAt          time.Time           `json:"created_at"`
	BusinessFinishedAt *time.Time          `json:"business_finished_at,omitempty"`
	ResultSnapshot     *GenerationProgress `json:"result_snapshot,omitempty"`
	ID                 string              `json:"id"`
	Commit             string              `json:"commit,omitempty"`
	Profile            string              `json:"profile,omitempty"`
	ProfileDigest      string              `json:"profile_digest,omitempty"`
	Status             string              `json:"status,omitempty"`
	Diagnostic         string              `json:"diagnostic,omitempty"`
	// Processed is the explicit maintenance command receipt, not a Run state.
	Processed   bool                  `json:"processed"`
	Items       []GenerationBatchItem `json:"items"`
	CompletedAt *time.Time            `json:"completed_at,omitempty"`
}

type GenerationBatchItem struct {
	TaskID       string `json:"task_id"`
	Inputs       string `json:"inputs"`
	GenerationID string `json:"generation_id"`
	Status       string `json:"status,omitempty"`
	Diagnostic   string `json:"diagnostic,omitempty"`
}

type GenerationProgress struct {
	ID          string     `json:"id"`
	Status      string     `json:"status,omitempty"`
	Diagnostic  string     `json:"diagnostic,omitempty"`
	Total       int        `json:"total"`
	Succeeded   int        `json:"succeeded"`
	Queued      int        `json:"queued"`
	Running     int        `json:"running"`
	Failed      int        `json:"failed"`
	Blocked     int        `json:"blocked"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}
