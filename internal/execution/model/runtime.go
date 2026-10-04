package model

import (
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

const DecisionRequestFormatVersion = 2

type PendingQuestion struct {
	Questions []executionbackend.Question                 `json:"questions"`
	Route     executionbackend.ImplementationDataEnvelope `json:"route"`
}

// A human-confirmed stop settles the resource uncertainty, not the outcome of
// any previously sent operation. It applies only to this exact attempt.
type ManualStopConfirmation struct {
	AttemptID   string    `json:"attempt_id"`
	ConfirmedAt time.Time `json:"confirmed_at"`
	ConfirmedBy string    `json:"confirmed_by,omitempty"`
	Basis       string    `json:"basis,omitempty"`
}

// DecisionRequest is the structured question carried in an R-a result.
type DecisionRequest struct {
	FormatVersion int                                          `json:"format_version"`
	Question      string                                       `json:"question"`
	Context       string                                       `json:"context,omitempty"`
	Options       []executionbackend.DecisionOption            `json:"options"`
	Questions     []executionbackend.Question                  `json:"questions,omitempty"`
	QuestionRoute *executionbackend.ImplementationDataEnvelope `json:"question_route,omitempty"`
	Recommended   string                                       `json:"recommended,omitempty"`
	AllowFreeText bool                                         `json:"allow_free_text,omitempty"`
}

// DecisionState is the runtime projection of one decision. SQLite owns the
// accepted request and answer; callers submit answers through CommandService.
type DecisionState struct {
	AnswerDecisionVersion uint64              `json:"answer_decision_version,omitempty"`
	AnswerRunRevision     uint64              `json:"answer_run_revision,omitempty"`
	ID                    string              `json:"id"`
	AttemptID             string              `json:"attempt_id,omitempty"`
	Version               uint64              `json:"version"`
	Request               DecisionRequest     `json:"request"`
	Status                string              `json:"status"`
	OptionID              string              `json:"option_id,omitempty"`
	Note                  string              `json:"note,omitempty"`
	Answers               map[string][]string `json:"answers,omitempty"`
	SourceAttemptID       string              `json:"source_attempt_id,omitempty"`
	RecoveryVersion       uint64              `json:"recovery_version,omitempty"`
	AnsweredBy            string              `json:"answered_by,omitempty"`
	AnsweredAt            *time.Time          `json:"answered_at,omitempty"`
	AppliedAt             *time.Time          `json:"applied_at,omitempty"`
	// Decoded only to reject projections written by the old sidecar-answer
	// implementation. These paths are never read as answer authority.
	RequestPath string `json:"request_path,omitempty"`
	AnswerPath  string `json:"answer_path,omitempty"`
}

// DecisionAnswer is an immutable human answer submitted to CommandService.
type DecisionAnswer struct {
	DecisionID              string              `json:"decision_id"`
	RunID                   string              `json:"run_id"`
	OptionID                string              `json:"option_id,omitempty"`
	Note                    string              `json:"note,omitempty"`
	Answers                 map[string][]string `json:"answers,omitempty"`
	AnsweredBy              string              `json:"answered_by"`
	AnsweredAt              time.Time           `json:"answered_at"`
	ExpectedDecisionVersion uint64              `json:"expected_decision_version,omitempty"`
	ExpectedRunRevision     uint64              `json:"expected_run_revision,omitempty"`
	SourceAttemptID         string              `json:"source_attempt_id,omitempty"`
	ExpectedRecoveryVersion uint64              `json:"expected_recovery_version,omitempty"`
	ServiceInstanceID       string              `json:"service_instance_id,omitempty"`
}

// Intent is durable before Git creates anything; actual workspace fields stay null
// until creation and verification succeed.
type PRState struct {
	URL          *string    `json:"url"`
	HeadRevision *string    `json:"head_revision,omitempty"`
	State        string     `json:"state"`
	MergedAt     *time.Time `json:"merged_at"`
	MergeCommit  *string    `json:"merge_commit"`
	QueriedAt    *time.Time `json:"queried_at"`
	QueryError   *string    `json:"query_error"`
}
