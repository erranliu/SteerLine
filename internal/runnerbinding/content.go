package runnerbinding

import (
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// Content contains caller facts only. Each business owner retains its record and CAS.
type Content struct {
	StateDir        string
	Request         runnercontract.Request
	RunID           string
	InputDigest     string
	Revision        uint64
	Project         string
	ObjectKey       string
	ObjectID        string
	WorkflowID      string
	WorkflowVersion int
	RequireDigest   bool
	Unsubmitted     bool
	Rejected        bool
}

func (c Content) Selector() (string, string) {
	if c.RunID == "" || c.InputDigest == "" {
		return "request-id", c.Request.RequestID
	}
	return "run-id", c.RunID
}

func ValidateContentCorrelation(c Content, run *runnercontract.Run) error {
	if run == nil || run.Correlation["steerline_project"] != c.Project || run.Correlation[c.ObjectKey] != c.ObjectID {
		return errors.New("original content correlation differs")
	}
	return nil
}

func ValidateContent(c Content, view runnercontract.Inspection) error {
	run := view.Run
	if run == nil {
		return errors.New("original content Run is missing")
	}
	selector, _ := c.Selector()
	if selector == "request-id" && (view.Command == nil || view.Command.Command != "submit" || view.Command.RequestID != c.Request.RequestID || view.Command.RunID == nil || *view.Command.RunID != run.RunID) {
		return errors.New("original content submit binding is unverified")
	}
	if err := ValidateContentCorrelation(c, run); err != nil {
		return err
	}
	if run.Workflow.ID != c.WorkflowID || run.Workflow.Version != c.WorkflowVersion || c.RunID != "" && c.RunID != run.RunID || c.InputDigest != "" && c.InputDigest != run.InputDigest || c.RequireDigest && run.InputDigest == "" {
		return errors.New("original content Run identity/input differs")
	}
	if run.Revision < c.Revision {
		return errors.New("original content observation revision moved backwards")
	}
	return nil
}

func NeedsContentStop(request *runnercontract.Request, state, stopStatus string) bool {
	return stopStatus != "stopped" && (request == nil || state == "rejected")
}

func ContentStop(requestID string, revision uint64) *runnercontract.Request {
	body, _ := json.Marshal(map[string]uint64{"expected_revision": revision})
	return &runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: requestID, Input: body}
}
