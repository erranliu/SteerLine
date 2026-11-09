// Package model owns immutable test definitions and arrangement snapshots.
// It has no Runner client, storage implementation or application dependencies.
package model

import (
	"errors"
)

var ErrNotFound = errors.New("test record not found")

type Definition struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Level       string `json:"level"`
	Environment string `json:"environment"`
	Command     string `json:"command"`
	Check       string `json:"check"`
	Note        string `json:"note,omitempty"`
}
type Catalogue struct {
	Tests []Definition `json:"tests"`
}

// Snapshot is the complete frozen arrangement; prompt views never replace it.
type Snapshot struct {
	Project     ProjectEntry `json:"project"`
	Status      string       `json:"status"`
	Diagnostic  string       `json:"diagnostic,omitempty"`
	PlanPath    string       `json:"plan_path"`
	Commit      string       `json:"commit"`
	Definitions []Definition `json:"definitions"`
	Digest      string       `json:"digest,omitempty"`
}
