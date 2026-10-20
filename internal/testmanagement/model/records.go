// Package model owns immutable test definitions and generation records.
// It has no Runner client, storage implementation or application dependencies.
package model

import (
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"time"
)

var ErrNotFound = errors.New("test generation not found")

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
type GenerateCommand struct {
	RequestID string `json:"request_id"`
	Plan      string `json:"plan"`
	Profile   string `json:"profile"`
}
type Source struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}
type Generation struct {
	ID           string                  `json:"id"`
	Project      string                  `json:"project"`
	Revision     uint64                  `json:"revision"`
	Command      GenerateCommand         `json:"command"`
	CreatedAt    time.Time               `json:"created_at"`
	Runner       runnercontract.BuildPin `json:"runner"`
	Request      runnercontract.Request  `json:"runner_request"`
	Sources      []Source                `json:"sources"`
	Target       string                  `json:"target"`
	TargetBefore string                  `json:"target_before"`
	Catalogue    Catalogue               `json:"catalogue"`
	RunID        string                  `json:"run_id,omitempty"`
	InputDigest  string                  `json:"input_digest,omitempty"`
	Projection   *runnercontract.Run     `json:"report,omitempty"`
	State        string                  `json:"state"`
	Diagnostic   string                  `json:"diagnostic,omitempty"`
	Result       json.RawMessage         `json:"result,omitempty"`
	Cancelled    bool                    `json:"cancelled"`
	StopID       string                  `json:"stop_id,omitempty"`
	StopRequest  *runnercontract.Request `json:"stop_request,omitempty"`
	StopState    string                  `json:"stop_state,omitempty"`
}
