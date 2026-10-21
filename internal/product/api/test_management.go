package api

import (
	"context"
	"encoding/json"
)

type TestGenerateRequest struct {
	RequestID string `json:"request_id"`
	Plan      string `json:"plan"`
	Profile   string `json:"profile"`
	Commit    string `json:"commit,omitempty"`
}
type TestStopGenerationRequest struct {
	RequestID    string `json:"request_id"`
	GenerationID string `json:"generation_id"`
}
type TestApplyRequest struct {
	RequestID        string `json:"request_id"`
	GenerationID     string `json:"generation_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	ReviewDigest     string `json:"review_digest"`
	AllowEmpty       bool   `json:"allow_empty"`
}
type TestGenerationReview struct {
	ReviewDigest string `json:"review_digest"`
	Before       string `json:"before"`
	After        string `json:"after"`
	Diagnostic   string `json:"diagnostic,omitempty"`
}

type TestGeneration struct {
	Applied      bool                  `json:"applied"`
	Plan         string                `json:"plan"`
	Profile      string                `json:"profile"`
	SourceCommit string                `json:"source_commit,omitempty"`
	Review       *TestGenerationReview `json:"review,omitempty"`
	ID           string                `json:"id"`
	Revision     uint64                `json:"revision"`
	State        string                `json:"state"`
	Target       string                `json:"target"`
	RunID        string                `json:"run_id,omitempty"`
	StopStatus   string                `json:"stop_status,omitempty"`
	Diagnostic   string                `json:"diagnostic,omitempty"`
	Result       json.RawMessage       `json:"result,omitempty"`
}
type TestManagementPort interface {
	ApplyTests(context.Context, TestApplyRequest) (TestGeneration, error)
	PlanTests(context.Context, string) (json.RawMessage, error)
	GenerateTests(context.Context, TestGenerateRequest) (TestGeneration, error)
	TestGeneration(context.Context, string) (TestGeneration, error)
	StopTestGeneration(context.Context, TestStopGenerationRequest) (TestGeneration, error)
}
