// Package model contains platform-independent rules for one resumable
// worker session. The center supplies verified facts and owns all writes.
package model

import (
	"errors"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"time"
)

const Version = "worker-flow-v2"

var ErrTurnStale = executionbackend.ErrTurnStale

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
