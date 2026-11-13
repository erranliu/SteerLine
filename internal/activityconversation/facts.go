// Package activityconversation carries source facts for the shared conversation reader.
package activityconversation

import "github.com/erranliu/SteerLine/internal/runnercontract"

// Facts are projected from one stored activity record.
type Facts struct {
	Runs       []Run
	Diagnostic string
	Operation  *Operation
}

type Run struct {
	Key, Label, RunID, StateDir string
	Projection                  *runnercontract.Run
}

type Operation struct {
	Label, RequestID, Status, Diagnostic string
}
