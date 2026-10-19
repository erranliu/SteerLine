package ports

import "github.com/erranliu/SteerLine/internal/runnercontract"

// ActivityBinding records only the selected execution and attempt ownership
// facts needed before projecting their bounded event page.
type ActivityBinding struct {
	ProjectIdentity        string
	ExecutionID            string
	ExecutionWorkflowRunID string
	AttemptID              string
	AttemptExecutionID     string
}

type ActivityRecord struct {
	EventID int64
	TaskID  string
	Event   runnercontract.Event
}

// ActivityRecords is a bounded, pinned read stream. Next returns nil at EOF;
// Close releases the read transaction, including on budget stops or errors.
type ActivityRecords interface {
	Next() (*ActivityRecord, error)
	Close() error
}
