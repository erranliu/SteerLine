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
)

type Error struct {
	Kind  ErrorKind
	Cause error
}

func (e *Error) Error() string { return e.Cause.Error() }

func (e *Error) Unwrap() error { return e.Cause }

type ControlAction string

const (
	OpenTerminal       ControlAction = "open_terminal"
	RecheckEnvironment ControlAction = "recheck_environment"
	StartTask          ControlAction = "start_task"
	StartCandidate     ControlAction = "start_candidate"
	AbandonCandidate   ControlAction = "abandon_candidate"
	SetDispatchMode    ControlAction = "set_dispatch_mode"
	PauseDispatch      ControlAction = "pause_dispatch"
	ResumeDispatch     ControlAction = "resume_dispatch"
	StopService        ControlAction = "stop_service"
)

type ControlInput struct {
	Action                             ControlAction
	Payload                            ControlPayload
	RequestID, RequestedBy, InstanceID string
	ExpectedRevision                   *uint64
}

type AnswerInput struct {
	RunID, DecisionID, Option, Note, RequestedBy, CommandID string
	ExpectedVersion                                         uint64
}

type CreateInput struct {
	Payload                  []byte
	RequestID, RequestedBy   string
	ExpectedAuthorityVersion uint64
}

type DiscardInput struct {
	TaskID, RequestID, Reason, RequestedBy string
	ExpectedVersion                        uint64
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
	RunID, ExecutionID, AttemptID string
	AfterEventID                  int64
	Initialized                   bool
}
