package model

import "github.com/erranliu/SteerLine/internal/taskcontract"

// Material is the latest observed input for one task. Candidates hold their own
// immutable contract; replacing this observation never changes an execution.
type Material struct {
	TaskID       string                    `json:"task_id"`
	Commit       string                    `json:"commit"`
	Contract     taskcontract.TaskContract `json:"contract"`
	Digest       string                    `json:"digest"`
	Inputs       string                    `json:"inputs"`
	GenerationID string                    `json:"generation_id"`
	Status       string                    `json:"status"`
	Diagnostic   string                    `json:"diagnostic,omitempty"`
}
