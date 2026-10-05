package execution

import (
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// ResourcePort is an injected OS/file fact capability; it has no business writes.
type ResourcePort interface {
	WorkerTreeAbsent(executionbackend.ProcessIdentity) bool
	AtomicWrite(string, []byte) error
}
