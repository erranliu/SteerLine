// Package model contains platform-independent rules for one resumable
// worker session. The center supplies verified facts and owns all writes.
package model

import (
	"errors"
	"fmt"
	"time"
)

const Version = "worker-flow-v2"

var ErrTurnStale = errors.New("worker turn is no longer current")

// State is a transient view of the authoritative workflow and execution.
type State struct {
	WorkflowRunID, TaskID, WorkflowVersion, Status string
	PRURL, PRState                                 string
	WorkflowUpdatedAt                              time.Time
	ExecutionID, ExecutionWorkflowRunID            string
	SessionID, PendingTurnID, CurrentTurnID        string
	LastStoppedTurnID, ExecutionStatus             string
	TurnNumber                                     uint64
	ExecutionUpdatedAt                             time.Time
}

func (s State) validBinding() error {
	if s.WorkflowVersion != Version || s.WorkflowRunID == "" || s.ExecutionID == "" || s.ExecutionWorkflowRunID != s.WorkflowRunID {
		return errors.New("worker flow has no fixed workflow and execution binding")
	}
	if s.Status == "completed" || s.Status == "abandoned" {
		return errors.New("terminal worker flow cannot resume")
	}
	return nil
}

func ValidateBinding(s State) error { return s.validBinding() }

func BeginTurn(s State, sessionID, turnID string, at time.Time) (State, error) {
	if err := s.validBinding(); err != nil {
		return s, err
	}
	if sessionID == "" || turnID == "" {
		return s, errors.New("worker turn start lacks native session or turn identity")
	}
	if s.PendingTurnID != "" && s.PendingTurnID != turnID {
		return s, errors.New("worker turn start differs from the persisted send intent")
	}
	if s.SessionID != "" && s.SessionID != sessionID {
		return s, errors.New("worker session replacement is not an ordinary continuation")
	}
	if s.CurrentTurnID == turnID {
		if s.LastStoppedTurnID == turnID {
			return s, ErrTurnStale
		}
		return s, nil
	}
	if s.CurrentTurnID != "" && s.LastStoppedTurnID != s.CurrentTurnID {
		return s, errors.New("previous worker turn has not stopped")
	}
	switch s.Status {
	case "queued", "waiting_input", "waiting_external":
	case "reconciling":
		// This is an observation of the original accepted input, not permission
		// to dispatch another input while its outcome is still unknown.
		if s.PendingTurnID != turnID {
			return s, errors.New("reconciling start has no matching original accepted input")
		}
	default:
		return s, fmt.Errorf("worker turn cannot start from %s", s.Status)
	}
	s.SessionID = sessionID
	s.PendingTurnID = ""
	s.CurrentTurnID = turnID
	s.TurnNumber++
	s.ExecutionStatus = "active"
	s.ExecutionUpdatedAt = at
	s.Status = "running"
	s.WorkflowUpdatedAt = at
	return s, nil
}

func NoteQuestion(s State, turnID string, at time.Time) (State, error) {
	if err := s.validBinding(); err != nil {
		return s, err
	}
	if turnID == "" || s.CurrentTurnID != turnID || s.LastStoppedTurnID == turnID {
		return s, ErrTurnStale
	}
	if s.Status != "running" && s.Status != "waiting_input" {
		return s, fmt.Errorf("worker question cannot arrive in %s", s.Status)
	}
	s.Status = "waiting_input"
	s.WorkflowUpdatedAt = at
	return s, nil
}

func ContinueQuestion(s State, turnID string, at time.Time) (State, error) {
	if err := s.validBinding(); err != nil {
		return s, err
	}
	if turnID == "" || s.CurrentTurnID != turnID || s.LastStoppedTurnID == turnID {
		return s, ErrTurnStale
	}
	if s.Status != "waiting_input" {
		return s, fmt.Errorf("worker question is not waiting in %s", s.Status)
	}
	s.Status = "running"
	s.WorkflowUpdatedAt = at
	return s, nil
}

func StopTurn(s State, turnID string, pendingQuestion bool, at time.Time) (State, error) {
	if err := s.validBinding(); err != nil {
		return s, err
	}
	if turnID == "" || s.CurrentTurnID != turnID {
		return s, ErrTurnStale
	}
	if s.LastStoppedTurnID == turnID {
		return s, nil
	}
	if s.Status != "running" && s.Status != "waiting_input" {
		return s, fmt.Errorf("worker turn cannot stop from %s", s.Status)
	}
	s.LastStoppedTurnID = turnID
	s.ExecutionStatus = "idle"
	s.ExecutionUpdatedAt = at
	// Native stop is not business completion. The bound flow decides what
	// follows after its result and artifact checks.
	s.Status = "blocked"
	if pendingQuestion {
		s.Status = "waiting_input"
	}
	s.WorkflowUpdatedAt = at
	return s, nil
}

// BindPR consumes a PR identity already checked by the provider adapter.
func BindPR(s State, url, state string, pendingQuestion bool, at time.Time) (State, error) {
	if err := s.validBinding(); err != nil {
		return s, err
	}
	if s.PRURL != "" && s.PRURL != url {
		return s, errors.New("another PR is already bound to this execution")
	}
	s.PRURL, s.PRState = url, state
	s.WorkflowUpdatedAt = at
	return s, nil
}

type Delivery struct {
	TaskID, WorkflowRunID, PRURL, MergeCommit string
	MergedAt                                  time.Time
}

// Complete applies a delivery only after the center committed it atomically.
func Complete(s State, taskID, taskStatus string, delivery Delivery, at time.Time) (State, error) {
	if s.WorkflowVersion != Version || taskStatus != "closed" || taskID != s.TaskID || delivery.TaskID != taskID ||
		delivery.WorkflowRunID != s.WorkflowRunID || delivery.PRURL == "" || s.PRURL != delivery.PRURL ||
		delivery.MergeCommit == "" || delivery.MergedAt.IsZero() {
		return s, errors.New("completed worker flow requires committed matching delivery")
	}
	if s.Status == "completed" {
		return s, nil
	}
	if s.Status == "abandoned" {
		return s, errors.New("discarded worker flow cannot become completed")
	}
	s.Status = "completed"
	s.PRState = "merged"
	s.WorkflowUpdatedAt = at
	return s, nil
}
