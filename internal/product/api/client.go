package api

import (
	"errors"
)

type ErrorKind string

var ErrRecordNotFound = errors.New("product record not found")

const (
	Unavailable        ErrorKind = "unavailable"
	MissingDatabase    ErrorKind = "missing_database"
	IncompatibleFormat ErrorKind = "incompatible_format"
	IdentityConflict   ErrorKind = "identity_conflict"
	UnknownRecord      ErrorKind = "unknown_record"
	ReadFailed         ErrorKind = "read_failed"
	Closed             ErrorKind = "closed"
	Unsupported        ErrorKind = "unsupported"
)

type Error struct {
	Kind    ErrorKind `json:"kind"`
	Message string    `json:"message"`
}

func (e *Error) Error() string { return e.Message }

type ControlAction string

const (
	OpenTerminal       ControlAction = "open_terminal"
	RecheckEnvironment ControlAction = "recheck_environment"
	StartTask          ControlAction = "start_task"
	StartCandidate     ControlAction = "start_candidate"
	StopCandidate      ControlAction = "stop_candidate"
	SetDispatchMode    ControlAction = "set_dispatch_mode"
	PauseDispatch      ControlAction = "pause_dispatch"
	ResumeDispatch     ControlAction = "resume_dispatch"
	StopService        ControlAction = "stop_service"
)

type ControlInput struct {
	Action           ControlAction  `json:"action"`
	Payload          ControlPayload `json:"payload"`
	RequestID        string         `json:"request_id"`
	RequestedBy      string         `json:"requested_by"`
	InstanceID       string         `json:"instance_id"`
	ExpectedRevision *uint64        `json:"expected_revision,omitempty"`
}

type AnswerInput struct {
	RunID           string `json:"run_id"`
	DecisionID      string `json:"decision_id"`
	Option          string `json:"option"`
	Note            string `json:"note"`
	RequestedBy     string `json:"requested_by"`
	CommandID       string `json:"command_id"`
	ExpectedVersion uint64 `json:"expected_version"`
}

type CreateInput struct {
	Payload                  []byte `json:"payload"`
	RequestID                string `json:"request_id"`
	RequestedBy              string `json:"requested_by"`
	ExpectedAuthorityVersion uint64 `json:"expected_authority_version"`
}

type DiscardInput struct {
	TaskID          string `json:"task_id"`
	RequestID       string `json:"request_id"`
	Reason          string `json:"reason"`
	RequestedBy     string `json:"requested_by"`
	ExpectedVersion uint64 `json:"expected_version"`
}

type ServiceStopResult struct {
	Status        string          `json:"status"`
	RequestID     string          `json:"request_id"`
	InstanceID    string          `json:"instance_id"`
	Receipt       *ControlReceipt `json:"receipt,omitempty"`
	ProcessExited bool            `json:"process_exited"`
	LocksReleased bool            `json:"locks_released"`
	Diagnostic    string          `json:"diagnostic,omitempty"`
}

// ActivityCursor belongs to one canonical database stream.
type ActivityCursor struct {
	RunID        string `json:"run_id"`
	ExecutionID  string `json:"execution_id"`
	AttemptID    string `json:"attempt_id"`
	AfterEventID int64  `json:"after_event_id"`
	Initialized  bool   `json:"initialized"`
}
