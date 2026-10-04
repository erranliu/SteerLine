package model

import (
	"strings"
)

// DiscardIntent records the human task decision. It says nothing about whether
// execution resources have stopped or whether provider reconciliation completed.
type DiscardIntent struct {
	TaskID          string `json:"task_id"`
	ExpectedVersion uint64 `json:"expected_version"`
	Reason          string `json:"reason"`
	RequestedBy     string `json:"requested_by"`
}

func ValidateDiscardIntent(v DiscardIntent) error {
	if ValidateID(v.TaskID) != nil || v.ExpectedVersion == 0 || strings.TrimSpace(v.RequestedBy) == "" || strings.TrimSpace(v.Reason) == "" || len(v.Reason) > 4000 {
		return newError("immutable_task", "discard requires a task version, reason and requester")
	}
	return nil
}
