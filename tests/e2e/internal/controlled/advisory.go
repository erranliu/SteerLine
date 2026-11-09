package controlled

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"os"
	"time"
)

// AdvisoryScript is external peer data, never application state. Results are
// indexed by original question so later questions cannot rewrite earlier Runs.
type AdvisoryScript struct {
	Question *runnercontract.Question    `json:"question"`
	Results  map[string]AdvisoryResponse `json:"results"`
}
type AdvisoryResponse struct {
	State       string `json:"state"`
	Prompt      string `json:"prompt"`
	LoseReceipt bool   `json:"lose_receipt"`
	Unavailable bool   `json:"unavailable"`
	HoldStop    bool   `json:"hold_stop"`
}

func readAdvisoryScript(provider string) (AdvisoryScript, error) {
	var s AdvisoryScript
	b, e := os.ReadFile(provider + ".advisory.json")
	if e != nil {
		return s, e
	}
	e = json.Unmarshal(b, &s)
	return s, e
}
func scriptedQuestion(provider string, r runnercontract.Run) (runnercontract.Run, error) {
	s, e := readAdvisoryScript(provider)
	if os.IsNotExist(e) {
		return r, nil
	}
	if e != nil {
		return r, e
	}
	if r.Workflow.ID != "development" || r.StopStatus == "stopped" {
		return r, nil
	}
	before, _ := json.Marshal(r.Question)
	after, _ := json.Marshal(s.Question)
	if string(before) != string(after) {
		r.Question = s.Question
		r.Controls.CanReply = s.Question != nil
		r.Revision++
		r.UpdatedAt = time.Now().UTC()
		r.Status, r.Phase = "running", "running"
		if s.Question != nil {
			r.Status, r.Phase = "waiting_input", "waiting_input"
		}
	}
	return r, nil
}
func (s *RunnerFixture) submitAdvisory(stateDir string, request runnercontract.Request) (any, string, error) {
	var in runnercontract.GenerateRunInput
	if e := json.Unmarshal(request.Input, &in); e != nil {
		return nil, "", e
	}
	provider := statePathForDirectory(stateDir)
	script, e := readAdvisoryScript(provider)
	if e != nil {
		return nil, "", e
	}
	if script.Question == nil || in.Correlation["steerline_advisory"] == "" || in.Generate.Repository != nil || len(in.Generate.Materials) != 0 || len(in.Generate.OutputSchema) != 0 || in.Execution.Adapter != "codex-app-server" || in.Execution.Target.Kind != "local" {
		return nil, "", fmt.Errorf("unexpected advisory wire")
	}
	var settings map[string]any
	_ = json.Unmarshal(in.Execution.Settings, &settings)
	if settings["access_mode"] != "read_only" {
		return nil, "", fmt.Errorf("advisory is not read-only")
	}
	id := "run-" + request.RequestID
	hash := sha256.Sum256(request.Input)
	run := runnercontract.Run{RunID: id, Revision: 1, InputDigest: hex.EncodeToString(hash[:]), Workflow: in.Workflow, Correlation: in.Correlation, Status: "running", Phase: "running", StopStatus: "active", UpdatedAt: time.Now().UTC(), Execution: runnercontract.Execution{AttemptID: "attempt-" + id, SessionID: "session-" + id, TurnID: "turn-" + id}}
	if s.AdvisoryQuestions == nil {
		s.AdvisoryQuestions = map[string]string{}
	}
	s.AdvisoryQuestions[id] = script.Question.ID
	s.Runs[id] = run
	s.Providers[id] = provider
	cmd := runnercontract.Command{CommandID: "command-" + request.RequestID, RequestID: request.RequestID, RunID: &id, Command: "submit", ReceiptStatus: "accepted", ActionStatus: "completed"}
	s.Requests[request.RequestID] = cmd
	s.Bodies[request.RequestID] = string(request.Input)
	code := ""
	if script.Results[script.Question.ID].LoseReceipt {
		code = "request_outcome_unknown"
	}
	return map[string]any{"command_id": cmd.CommandID, "run_id": id, "receipt_status": "accepted", "action_status": "completed"}, code, nil
}
func (s *RunnerFixture) observeAdvisory(id string, offline bool) (runnercontract.Run, string, error) {
	r := s.Runs[id]
	script, e := readAdvisoryScript(s.Providers[id])
	if e != nil {
		return r, "", e
	}
	response := script.Results[s.AdvisoryQuestions[id]]
	if s.AdvisoryInspects == nil {
		s.AdvisoryInspects = map[string]int{}
	}
	s.AdvisoryInspects[id]++
	if response.Unavailable && !offline {
		if s.AdvisoryQueryFailures == nil {
			s.AdvisoryQueryFailures = map[string]int{}
		}
		s.AdvisoryQueryFailures[id]++
		return r, "fixture_query_unavailable", nil
	}
	before, _ := json.Marshal(r)
	if s.AdvisoryStops[id] {
		if !response.HoldStop {
			r.Status, r.StopStatus, r.Phase, r.Closed = "stopped", "stopped", "closed", true
		}
	} else if response.State == "ready" || response.State == "failed" {
		r.Status, r.StopStatus, r.Phase, r.Closed = "completed", "stopped", "closed", true
		final := runnercontract.Final{CommandID: "final-" + id, AttemptID: r.Execution.AttemptID, SessionID: r.Execution.SessionID, TurnID: r.Execution.TurnID, NativeOutcome: "succeeded", Verified: true, Source: "fixture.final", ItemID: "item-" + id, Text: response.Prompt}
		r.Final = &final
		r.ContentResult = &runnercontract.ContentResult{Status: "succeeded", Text: response.Prompt, Source: final}
		if response.State == "failed" {
			r.Status = "failed"
			r.ContentResult.Status = "failed"
			r.ContentResult.Diagnostic = "scripted generation failed"
		}
	}
	after, _ := json.Marshal(r)
	if string(before) != string(after) {
		r.Revision++
		r.UpdatedAt = time.Now().UTC()
	}
	return r, "", nil
}
