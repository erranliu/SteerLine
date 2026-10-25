package query

import (
	"github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func runnerDelivery(r *runnercontract.DeliveryObservation) *api.RunnerDeliverySummary {
	v := &api.RunnerDeliverySummary{Available: r.Available, RunID: r.RunID, Revision: r.Revision, Submission: r.Submission, Step: r.Step, Status: r.Status, Phase: r.Phase, Diagnostic: r.Diagnostic, NativeOutcome: r.NativeOutcome, FinalVerified: r.FinalVerified, ContentStatus: r.ContentStatus, PRState: r.PRState, PRQueryError: r.PRQueryError, StopStatus: r.StopStatus, Closed: r.Closed, ResumeRequired: r.ResumeRequired, NewInputsAllowed: r.NewInputsAllowed, Commands: []api.RunnerCommandSummary{}}
	if w := r.ExecutionWait; w != nil {
		v.ExecutionWait = &api.ExecutionWaitSummary{Contract: w.Contract, OperationID: w.OperationID, Query: w.Query, Stage: w.Stage, ReasonCode: w.ReasonCode, StartedAt: w.StartedAt, UpdatedAt: w.UpdatedAt, DeadlineAt: w.DeadlineAt, CallsStarted: w.CallsStarted, MaxCalls: w.MaxCalls, NextAttemptAt: copyValue(w.NextAttemptAt), Diagnostic: w.Diagnostic}
	}
	v.Controls = api.RunnerDeliveryControls{CanReply: r.Controls.CanReply, CanResume: r.Controls.CanResume, CanStop: r.Controls.CanStop, CanMessage: r.Controls.CanMessage, CanInterrupt: r.Controls.CanInterrupt, CanClose: r.Controls.CanClose, Reason: r.Controls.Reason}
	v.PRCheckedAt = copyValue(r.PRCheckedAt)
	for _, c := range r.Commands {
		v.Commands = append(v.Commands, api.RunnerCommandSummary{Command: c.Command, RequestID: c.RequestID, CommandID: c.CommandID, ReceiptState: c.ReceiptState, ActionStatus: c.ActionStatus})
	}
	return v
}
