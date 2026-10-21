package api

import (
	"context"
	"encoding/json"
)

type TestStopGenerationRequest struct {
	RequestID    string `json:"request_id"`
	GenerationID string `json:"generation_id"`
}
type TestGeneration struct {
	Plan         string          `json:"plan"`
	Profile      string          `json:"profile"`
	SourceCommit string          `json:"source_commit,omitempty"`
	ID           string          `json:"id"`
	Revision     uint64          `json:"revision"`
	State        string          `json:"state"`
	Target       string          `json:"target"`
	RunID        string          `json:"run_id,omitempty"`
	StopStatus   string          `json:"stop_status,omitempty"`
	Diagnostic   string          `json:"diagnostic,omitempty"`
	Result       json.RawMessage `json:"result,omitempty"`
}
type TestManagementPort interface {
	PlanTests(context.Context, string) (json.RawMessage, error)
	TestGeneration(context.Context, string) (TestGeneration, error)
	StopTestGeneration(context.Context, TestStopGenerationRequest) (TestGeneration, error)
}
