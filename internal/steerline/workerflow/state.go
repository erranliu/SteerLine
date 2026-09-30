// Package workerflow contains platform-independent rules for one resumable
// worker session. The center supplies verified facts and owns all writes.
package workerflow

import (
	"errors"
	"fmt"
	"time"
)

const Version = "worker-flow-v1"

var ErrTurnStale = errors.New("worker turn is no longer current")

// State is a transient view of the authoritative workflow and execution.
type State struct {
	WorkflowRunID, TaskID, WorkflowVersion, Phase string
	PRURL, PRState                                string
	WorkflowUpdatedAt                             time.Time
	ExecutionID, ExecutionWorkflowRunID           string
	SessionID, PendingTurnID, CurrentTurnID       string
	LastStoppedTurnID, ExecutionStatus            string
	TurnNumber                                    uint64
	ExecutionUpdatedAt                            time.Time
}

func (s State) validBinding() error {
	if s.WorkflowVersion != Version || s.WorkflowRunID == "" || s.ExecutionID == "" || s.ExecutionWorkflowRunID != s.WorkflowRunID {
		return errors.New("worker flow has no fixed workflow and execution binding")
	}
	if s.Phase == "completed" || s.Phase == "abandoned" {
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
	switch s.Phase {
	case "queued", "waiting_decision", "waiting_pr_merge":
	default:
		return s, fmt.Errorf("worker turn cannot start from %s", s.Phase)
	}
	s.SessionID = sessionID
	s.PendingTurnID = ""
	s.CurrentTurnID = turnID
	s.TurnNumber++
	s.ExecutionStatus = "active"
	s.ExecutionUpdatedAt = at
	s.Phase = "executing"
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
	if s.Phase != "executing" && s.Phase != "waiting_decision" {
		return s, fmt.Errorf("worker question cannot arrive in %s", s.Phase)
	}
	s.Phase = "waiting_decision"
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
	if s.Phase != "waiting_decision" {
		return s, fmt.Errorf("worker question is not waiting in %s", s.Phase)
	}
	s.Phase = "executing"
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
	if s.Phase != "executing" && s.Phase != "waiting_decision" {
		return s, fmt.Errorf("worker turn cannot stop from %s", s.Phase)
	}
	s.LastStoppedTurnID = turnID
	s.ExecutionStatus = "idle"
	s.ExecutionUpdatedAt = at
	if pendingQuestion || s.PRURL == "" || s.PRState != "open" && s.PRState != "merged" {
		s.Phase = "waiting_decision"
	} else {
		s.Phase = "waiting_pr_merge"
	}
	s.WorkflowUpdatedAt = at
	return s, nil
}

// BindPR consumes a PR identity already checked by the provider adapter.
func BindPR(s State, url, state string, pendingQuestion bool, at time.Time) (State, error) {
	if err := s.validBinding(); err != nil {
		return s, err
	}
	if s.PRURL != "" && s.PRURL != url && s.PRState != "closed_unmerged" {
		return s, errors.New("another PR is already bound to this execution")
	}
	s.PRURL, s.PRState = url, state
	if s.CurrentTurnID != "" && s.LastStoppedTurnID != s.CurrentTurnID {
		if s.Phase != "waiting_decision" || !pendingQuestion {
			s.Phase = "executing"
		}
	} else if pendingQuestion || state == "closed_unmerged" {
		s.Phase = "waiting_decision"
	} else {
		s.Phase = "waiting_pr_merge"
	}
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
	if s.Phase == "completed" {
		return s, nil
	}
	if s.Phase == "abandoned" {
		return s, errors.New("discarded worker flow cannot become completed")
	}
	s.Phase = "completed"
	s.PRState = "merged"
	s.WorkflowUpdatedAt = at
	return s, nil
}
