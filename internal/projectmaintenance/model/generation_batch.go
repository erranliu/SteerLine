package model

import "time"

// A batch freezes only membership. Generation records remain the result owner.
type GenerationBatch struct {
	ID string `json:"id"`
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
	Total       int        `json:"total"`
	Succeeded   int        `json:"succeeded"`
	Queued      int        `json:"queued"`
	Running     int        `json:"running"`
	Failed      int        `json:"failed"`
	Blocked     int        `json:"blocked"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}
