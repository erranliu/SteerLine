package model

import (
	"errors"
	"fmt"
	"time"
)

func ValidStatus(v string) bool {
	switch v {
	case "open", "in_progress", "discarded", "closed":
		return true
	}
	return false
}

// SetStatus applies one task lifecycle transition at the caller's timestamp.
func SetStatus(record *TaskRecord, next string, mergedPR bool, at time.Time) error {
	if record == nil {
		return errors.New("task is required")
	}
	if !ValidStatus(next) {
		return fmt.Errorf("invalid task status %q", next)
	}
	allowed := map[string]map[string]bool{"open": {"in_progress": true, "discarded": true}, "in_progress": {"discarded": true, "closed": true}, "discarded": {}, "closed": {}}
	if record.Status == next {
		return nil
	}
	if !allowed[record.Status][next] {
		return fmt.Errorf("task transition %s -> %s is not allowed", record.Status, next)
	}
	if next == "discarded" && mergedPR {
		return errors.New("task with a merged pull request cannot be discarded")
	}
	record.Status = next
	record.Version++
	record.UpdatedAt = at
	return nil
}
