package taskrunner

import (
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
)

func deliveryObservation(c runnerstate.Candidate) *runnercontract.DeliveryObservation {
	v := &runnercontract.DeliveryObservation{Submission: c.SubmitState, Commands: []runnercontract.CommandObservation{}}
	// Command receipts have their own observation time. They never establish a
	// Run completion or authorize a transition in this read projection.
	add := func(a *runnerstate.Action) {
		if a != nil {
			v.Commands = append(v.Commands, runnercontract.CommandObservation{Command: a.Command, RequestID: a.Request.RequestID, CommandID: a.CommandID, ReceiptState: a.State, ActionStatus: a.ActionStatus})
		}
	}
	if c.Submit.RequestID != "" {
		v.Commands = append(v.Commands, runnercontract.CommandObservation{Command: "submit", RequestID: c.Submit.RequestID, CommandID: c.SubmitCommandID, ReceiptState: c.SubmitState})
	}
	start := max(0, len(c.Actions)-4)
	for i := start; i < len(c.Actions); i++ {
		add(&c.Actions[i])
	}
	add(c.Stop)
	r := c.Projection
	if r == nil {
		return v
	}
	if c.RunID == "" || r.RunID != c.RunID || r.Revision == 0 || r.InputDigest == "" || c.InputDigest != "" && c.InputDigest != r.InputDigest {
		v.Diagnostic = "原 Run 观察身份或 revision 未核实"
		return v
	}
	v.Available, v.RunID, v.Revision = true, r.RunID, r.Revision
	v.Step, v.Status, v.Phase, v.Diagnostic = r.Step, r.Status, r.Phase, r.Diagnostic
	v.NativeOutcome = r.Execution.NativeOutcome
	if r.Final != nil && r.Final.AttemptID == r.Execution.AttemptID && r.Final.SessionID == r.Execution.SessionID && r.Final.TurnID == r.Execution.TurnID && r.Final.TurnID != "" {
		v.FinalVerified = r.Final.Verified
	}
	if r.ContentResult != nil {
		source := r.ContentResult.Source
		if source.AttemptID == r.Execution.AttemptID && source.SessionID == r.Execution.SessionID && source.TurnID == r.Execution.TurnID && source.TurnID != "" {
			v.ContentStatus = r.ContentResult.Status
		}
	}
	if r.PR != nil {
		v.PRState, v.PRQueryError = r.PR.State, r.PR.QueryError
		if !r.PR.CheckedAt.IsZero() {
			at := r.PR.CheckedAt
			v.PRCheckedAt = &at
		}
	}
	v.StopStatus, v.Closed = r.StopStatus, r.Closed
	v.ResumeRequired, v.NewInputsAllowed, v.Controls = r.ResumeRequired, r.NewInputsAllowed, r.Controls
	if c.StopRequested || c.Participation != "active" {
		v.Controls = runnercontract.Controls{Reason: "候选已停止参与或正在停止，仅可查看原事实"}
	}
	return v
}
