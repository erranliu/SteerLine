package runnercontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"
)

const ExecutionWaitContract = "steerrunner-execution-wait/v1"

// ExecutionWait describes a read-only preparation query, never input authority.
type ExecutionWait struct {
	Contract      string     `json:"contract"`
	OperationID   string     `json:"operation_id"`
	Query         string     `json:"query"`
	Stage         string     `json:"stage"`
	ReasonCode    string     `json:"reason_code"`
	StartedAt     time.Time  `json:"started_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	DeadlineAt    time.Time  `json:"deadline_at"`
	CallsStarted  int        `json:"calls_started"`
	MaxCalls      int        `json:"max_calls"`
	NextAttemptAt *time.Time `json:"next_attempt_at"`
	Diagnostic    string     `json:"diagnostic"`
}

func (w *ExecutionWait) Validate() error {
	if w == nil {
		return nil
	}
	invalid := errors.New("Runner preparation wait contract is invalid")
	if w.Contract != ExecutionWaitContract || w.OperationID == "" || (w.Query != "repository_inventory" && w.Query != "runtime_status") || w.MaxCalls != 5 || w.CallsStarted < 0 || w.CallsStarted > w.MaxCalls || len(w.Diagnostic) > 1024 {
		return invalid
	}
	for _, at := range []time.Time{w.StartedAt, w.UpdatedAt, w.DeadlineAt} {
		_, offset := at.Zone()
		if at.IsZero() || offset != 0 {
			return invalid
		}
	}
	if w.UpdatedAt.Before(w.StartedAt) || !w.DeadlineAt.After(w.StartedAt) || w.DeadlineAt.Sub(w.StartedAt) > 90*time.Second {
		return invalid
	}
	if w.Stage != "backoff" && w.NextAttemptAt != nil {
		return invalid
	}
	switch w.Stage {
	case "queue":
		if w.ReasonCode != "orca_cli_busy" || w.CallsStarted == w.MaxCalls {
			return invalid
		}
	case "calling":
		if w.ReasonCode != "orca_query_running" || w.CallsStarted == 0 {
			return invalid
		}
	case "backoff":
		if w.ReasonCode != "orca_query_transient" || w.CallsStarted == 0 || w.CallsStarted == w.MaxCalls || w.NextAttemptAt == nil || w.NextAttemptAt.Before(w.UpdatedAt) || !w.NextAttemptAt.Before(w.DeadlineAt) {
			return invalid
		}
		_, offset := w.NextAttemptAt.Zone()
		if offset != 0 {
			return invalid
		}
	case "exhausted":
		if w.ReasonCode != "wait_budget_exhausted" && w.ReasonCode != "query_attempts_exhausted" || w.ReasonCode == "query_attempts_exhausted" && w.CallsStarted != w.MaxCalls {
			return invalid
		}
	default:
		return invalid
	}
	return nil
}

func (w *ExecutionWait) UnmarshalJSON(raw []byte) error {
	canonical, err := CanonicalJSON(raw)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(canonical, &fields) != nil || len(fields) != 12 {
		return errors.New("Runner preparation wait requires every contract field")
	}
	for _, name := range []string{"contract", "operation_id", "query", "stage", "reason_code", "started_at", "updated_at", "deadline_at", "calls_started", "max_calls", "next_attempt_at", "diagnostic"} {
		if value, ok := fields[name]; !ok || name != "next_attempt_at" && bytes.Equal(value, []byte("null")) {
			return errors.New("Runner preparation wait has a missing or null field")
		}
	}
	type value ExecutionWait
	var decoded value
	d := json.NewDecoder(bytes.NewReader(canonical))
	d.DisallowUnknownFields()
	if err := d.Decode(&decoded); err != nil {
		return err
	}
	if err := (*ExecutionWait)(&decoded).Validate(); err != nil {
		return err
	}
	*w = ExecutionWait(decoded)
	return nil
}

func (w *ExecutionWait) Clone() *ExecutionWait {
	if w == nil {
		return nil
	}
	copy := *w
	if w.NextAttemptAt != nil {
		at := *w.NextAttemptAt
		copy.NextAttemptAt = &at
	}
	return &copy
}

// Wait validates only the new event variant, retaining absent versus cleared.
func (e Event) Wait() (*ExecutionWait, error) {
	if e.Type != "execution_wait_changed" {
		if len(e.ExecutionWait) != 0 {
			return nil, errors.New("Runner wait payload belongs to a different event type")
		}
		return nil, nil
	}
	if len(e.ExecutionWait) == 0 {
		return nil, errors.New("Runner wait event has no value or explicit null")
	}
	var wait *ExecutionWait
	if err := json.Unmarshal(e.ExecutionWait, &wait); err != nil {
		return nil, err
	}
	return wait, nil
}
