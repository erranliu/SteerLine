package model

import (
	"encoding/json"
	"time"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// Generation owns only one original Runner execution. Shared planning and
// publication remain on the document record.
type Generation struct {
	ID            string                  `json:"generation_id"`
	Key           string                  `json:"local_key"`
	CreatedAt     time.Time               `json:"created_at"`
	RetryRequest  string                  `json:"retry_request,omitempty"`
	State         string                  `json:"state"`
	Diagnostic    string                  `json:"diagnostic,omitempty"`
	Submission    string                  `json:"submission"`
	Request       runnercontract.Request  `json:"-"`
	RunID         string                  `json:"run_id,omitempty"`
	InputDigest   string                  `json:"input_digest,omitempty"`
	Projection    *runnercontract.Run     `json:"-"`
	Result        json.RawMessage         `json:"-"`
	StopRequest   *runnercontract.Request `json:"-"`
	StopState     string                  `json:"stop_state,omitempty"`
	Questions     []string                `json:"questions,omitempty"`
	Document      *Documents              `json:"document,omitempty"`
	Investigation []Investigation         `json:"investigation,omitempty"`
}
type StoredGeneration struct {
	Generation
	Result      json.RawMessage         `json:"result,omitempty"`
	Request     runnercontract.Request  `json:"runner_request"`
	Projection  *runnercontract.Run     `json:"projection,omitempty"`
	StopRequest *runnercontract.Request `json:"stop_request,omitempty"`
}

// Execution supplies the existing single-Run functions a temporary view. Its
// identity stays the parent record; only GenerationID selects the stored item.
func (r Record) Execution(g Generation) Record {
	r.GenerationID = g.ID
	r.CreatedAt, r.Request, r.Submission = g.CreatedAt, g.Request, g.Submission
	r.RunID, r.InputDigest, r.Projection = g.RunID, g.InputDigest, g.Projection
	r.State, r.Diagnostic, r.Result = g.State, g.Diagnostic, g.Result
	r.StopRequest, r.StopState = g.StopRequest, g.StopState
	r.Questions, r.Investigation = g.Questions, g.Investigation
	r.Documents = nil
	if g.Document != nil {
		r.Documents = []Documents{*g.Document}
	}
	return r
}
func (r Record) Executions() []Record {
	if r.Purpose != "documents" {
		return []Record{r}
	}
	out := make([]Record, 0, len(r.Generations))
	for _, g := range r.Generations {
		out = append(out, r.Execution(g))
	}
	return out
}
func (r Record) TargetKey() string {
	for _, g := range r.Generations {
		if g.ID == r.GenerationID {
			return g.Key
		}
	}
	return ""
}
func (r Record) CurrentGenerations() []Generation {
	latest := map[string]Generation{}
	for _, g := range r.Generations {
		latest[g.Key] = g
	}
	out := []Generation{}
	for _, d := range PlannedDocuments(r) {
		if g, ok := latest[d.Key]; ok {
			out = append(out, g)
		}
	}
	return out
}
func ConfirmationDigest(r Record) string {
	return ValueDigest(struct {
		Analysis  *Analysis
		Materials []runnercontract.Document
	}{r.Analysis, r.PlanningMaterials})
}
