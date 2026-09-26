package remoteprotocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var fullCommit = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// VerifyExpectation is the center's durable state, never inferred from a worker summary.
type VerifyExpectation struct {
	ProjectID       string
	TaskID          string
	WorkflowRunID   string
	ExecutionID     string
	AttemptID       string
	OperationID     string
	OrcaTaskID      string
	OrcaDispatchID  string
	InputRevision   string
	WorktreeID      string
	WorktreeBranch  string
	CandidateCommit string
	ObservedHead    string // Read from the execution host, not copied from the worker result.
	RequiredChecks  []string
}

// WorkerDone is the accepted Orca settlement that a structured result must match.
type WorkerDone struct {
	TaskID     string `json:"taskId"`
	DispatchID string `json:"dispatchId"`
	Outcome    string `json:"outcome"`
}

// VerifyCheck is one script receipt. Pointers distinguish missing values from
// exit code zero and an explicit false result.
type VerifyCheck struct {
	ID            string `json:"id"`
	Command       string `json:"command"`
	ScriptVersion string `json:"script_version"`
	ExitCode      *int   `json:"exit_code"`
	Passed        *bool  `json:"passed"`
	EvidenceRef   string `json:"evidence_ref"`
	ActualCommit  string `json:"actual_commit"`
}

// VerifyResult is the Spec 01 logical result for one verify step. It is bound
// to the native workflow, execution, attempt and Operation identities.
type VerifyResult struct {
	ProtocolVersion    int           `json:"protocol_version"`
	ProjectID          string        `json:"project_id"`
	TaskID             string        `json:"task_id"`
	WorkflowRunID      string        `json:"workflow_run_id"`
	ExecutionID        string        `json:"execution_id"`
	StepID             string        `json:"step_id"`
	AttemptID          string        `json:"attempt_id"`
	OperationID        string        `json:"operation_id"`
	OrcaTaskID         string        `json:"orca_task_id"`
	OrcaDispatchID     string        `json:"orca_dispatch_id"`
	ResultID           string        `json:"result_id"`
	Outcome            string        `json:"outcome"`
	Summary            string        `json:"summary"`
	Error              *string       `json:"error"`
	InputRevision      string        `json:"input_revision"`
	OutputRevision     string        `json:"output_revision"`
	WorktreeID         string        `json:"worktree_identity"`
	WorktreeBranch     string        `json:"worktree_branch"`
	CandidateCommit    string        `json:"candidate_commit"`
	AppliedDecisionIDs []string      `json:"applied_decision_ids"`
	Checks             []VerifyCheck `json:"checks"`
}

// ValidateVerifyGate accepts a verify result only when the exact settled worker,
// expected candidate commit and every required script receipt prove success.
// It is a pure guard for a future center adapter; it starts no workers and
// advances no workflow state.
func ValidateVerifyGate(raw []byte, done WorkerDone, want VerifyExpectation) error {
	if err := validateExpectation(want); err != nil {
		return err
	}
	if done.TaskID != want.OrcaTaskID || done.DispatchID != want.OrcaDispatchID {
		return errors.New("worker_done identity mismatch")
	}
	if done.Outcome != "succeeded" {
		return fmt.Errorf("worker_done outcome %q does not pass verify", done.Outcome)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("decode verify result: %w", err)
	}
	for _, field := range []string{
		"protocol_version", "project_id", "task_id", "workflow_run_id", "execution_id",
		"step_id", "attempt_id", "operation_id", "orca_task_id", "orca_dispatch_id",
		"result_id", "outcome", "summary", "error", "input_revision", "output_revision",
		"worktree_identity", "worktree_branch", "candidate_commit", "applied_decision_ids", "checks",
	} {
		if _, ok := fields[field]; !ok {
			return fmt.Errorf("verify result lacks %s", field)
		}
	}

	var result VerifyResult
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&result); err != nil {
		return fmt.Errorf("decode verify result: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("verify result contains trailing JSON")
	}
	if result.ProtocolVersion != 1 {
		return fmt.Errorf("unsupported verify protocol version %d", result.ProtocolVersion)
	}
	for _, pair := range []struct{ field, got, expected string }{
		{"project_id", result.ProjectID, want.ProjectID},
		{"task_id", result.TaskID, want.TaskID},
		{"workflow_run_id", result.WorkflowRunID, want.WorkflowRunID},
		{"execution_id", result.ExecutionID, want.ExecutionID},
		{"attempt_id", result.AttemptID, want.AttemptID},
		{"operation_id", result.OperationID, want.OperationID},
		{"orca_task_id", result.OrcaTaskID, want.OrcaTaskID},
		{"orca_dispatch_id", result.OrcaDispatchID, want.OrcaDispatchID},
		{"input_revision", result.InputRevision, want.InputRevision},
		{"worktree_identity", result.WorktreeID, want.WorktreeID},
		{"worktree_branch", result.WorktreeBranch, want.WorktreeBranch},
	} {
		if pair.got != pair.expected {
			return fmt.Errorf("verify %s mismatch", pair.field)
		}
	}
	if result.StepID != "verify" {
		return errors.New("verify step_id mismatch")
	}
	if result.Outcome != "succeeded" {
		return fmt.Errorf("verify outcome %q does not pass", result.Outcome)
	}
	if result.Error != nil && strings.TrimSpace(*result.Error) != "" {
		return errors.New("verify success contains error")
	}
	if strings.TrimSpace(result.ResultID) == "" || strings.TrimSpace(result.OutputRevision) == "" || strings.TrimSpace(result.Summary) == "" {
		return errors.New("verify result lacks result_id, output_revision or summary")
	}
	if result.AppliedDecisionIDs == nil {
		return errors.New("verify result lacks applied_decision_ids")
	}
	if !sameCommit(result.CandidateCommit, want.CandidateCommit) {
		return errors.New("verify candidate_commit mismatch")
	}
	if !sameCommit(want.ObservedHead, want.CandidateCommit) {
		return errors.New("observed worktree HEAD differs from candidate commit")
	}

	seen := make(map[string]bool, len(result.Checks))
	for _, check := range result.Checks {
		if strings.TrimSpace(check.ID) == "" || seen[check.ID] {
			return fmt.Errorf("verify check ID missing or duplicated: %q", check.ID)
		}
		seen[check.ID] = true
		if strings.TrimSpace(check.Command) == "" || strings.TrimSpace(check.ScriptVersion) == "" || strings.TrimSpace(check.EvidenceRef) == "" {
			return fmt.Errorf("verify check %q lacks command, script version or evidence", check.ID)
		}
		if check.ExitCode == nil || *check.ExitCode != 0 || check.Passed == nil || !*check.Passed {
			return fmt.Errorf("verify check %q did not pass", check.ID)
		}
		if !sameCommit(check.ActualCommit, want.CandidateCommit) {
			return fmt.Errorf("verify check %q commit mismatch", check.ID)
		}
	}
	for _, id := range want.RequiredChecks {
		if !seen[id] {
			return fmt.Errorf("verify required check %q missing", id)
		}
	}
	return nil
}

func validateExpectation(want VerifyExpectation) error {
	for _, value := range []string{
		want.ProjectID, want.TaskID, want.WorkflowRunID, want.ExecutionID,
		want.AttemptID, want.OperationID, want.OrcaTaskID, want.OrcaDispatchID,
		want.InputRevision, want.WorktreeID, want.WorktreeBranch,
	} {
		if strings.TrimSpace(value) == "" {
			return errors.New("incomplete verify expectation")
		}
	}
	if !fullCommit.MatchString(want.CandidateCommit) || !fullCommit.MatchString(want.ObservedHead) {
		return errors.New("verify expectation requires full candidate and observed HEAD commits")
	}
	if len(want.RequiredChecks) == 0 {
		return errors.New("verify expectation lacks required checks")
	}
	seen := make(map[string]bool, len(want.RequiredChecks))
	for _, id := range want.RequiredChecks {
		if strings.TrimSpace(id) == "" || seen[id] {
			return errors.New("verify expectation has missing or duplicate check ID")
		}
		seen[id] = true
	}
	return nil
}

func sameCommit(a, b string) bool {
	return fullCommit.MatchString(a) && fullCommit.MatchString(b) && strings.EqualFold(a, b)
}
