package taskrunner

import (
	"encoding/json"

	model "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// Project reconstructs a caller view from public CLI facts. It never invents
// native process identities or writes legacy execution records.
func Project(snapshot storageports.RunnerSnapshot, checkpoint storageports.ServiceCheckpoint) reconstruction.Runtime {
	g := snapshot.Graph
	view := reconstruction.RuntimeFromCheckpoint(checkpoint, g.Authority.DispatchMode, g.Authority.DispatchPaused, g.Authority.AuthorityVersion)
	view.Tasks = g.Tasks
	view.Runs = []*reconstruction.RunView{}
	for _, c := range snapshot.Candidates {
		v := projectCandidate(c)
		if task, ok := g.ByID[c.TaskID]; ok {
			v.Title = task.Title
			v.TaskStatus = task.Status
		}
		view.Runs = append(view.Runs, v)
		if c.Participation == "active" && c.SubmitState != "rejected" && c.SubmitState != "cancelled" && (c.Projection == nil || c.Projection.Status != "pr_ready" && c.Projection.Status != "stopped") {
			view.SlotOccupied = true
			if view.CurrentTask == nil {
				id := c.TaskID
				view.CurrentTask = &id
			}
		}
	}
	for _, d := range g.Deliveries {
		view.Deliveries = append(view.Deliveries, d)
	}
	return view
}

func text(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
func projectCandidate(c runnerbridge.Candidate) *reconstruction.RunView {
	v := &reconstruction.RunView{RunID: c.ID, TaskID: c.TaskID, DomainTaskID: c.TaskID, ExecutionID: c.RunID, ControlRevision: c.Revision, Claimant: c.Claimant, CandidateNumber: c.Number, CandidateCreatedAt: c.CreatedAt, CandidateTiming: "wall_since_reservation", ExecutionStatus: c.SubmitState, BaseCommit: text(c.BaseRevision), Error: text(c.Diagnostic), PR: model.PRState{State: "none"}}
	var input runnerclient.RunInput
	_ = json.Unmarshal(c.Submit.Input, &input)
	v.Host = "local"
	if input.Execution.Adapter == "orca-terminal" {
		v.Host = "orca"
		v.Target.ID = input.Execution.Target.Environment
	}
	v.Agent = "codex"
	v.FlowRef = input.Workflow.ID
	v.WorkflowVersion = "steerrunner-cli/v1"
	if c.StopRequested {
		v.ResourceStatus = "stop_pending"
	}
	if c.InputsRevoked {
		v.ResourceStatus = "inputs_revoked"
	}
	r := c.Projection
	if r == nil {
		return v
	}
	v.Backend, v.Protocol = r.Execution.Backend, r.Execution.Protocol
	v.SessionID, v.SessionGeneration = r.Execution.SessionID, r.Execution.Generation
	v.AttemptID, v.CurrentTurnID = r.Execution.AttemptID, r.Execution.TurnID
	v.WorktreePath, v.WorktreeBranch = text(r.Execution.Workspace), text(r.Execution.Branch)
	v.WorkflowStage, v.WorkflowStatus = r.Step, r.Status
	if c.Participation == "abandoned" {
		v.WorkflowStatus = "abandoned"
	}
	if c.Participation == "completed" {
		v.WorkflowStatus = "delivered"
	}
	v.ExecutionStatus, v.StageDiagnostic = r.Status, r.Diagnostic
	v.Summary, v.Error = text(r.Summary), text(r.Diagnostic)
	v.RecoveryRequired, v.RecoveryVersion = r.ResumeRequired, r.Revision
	if r.ResumeRequired {
		v.RecoveryState = "requires_explicit_resume"
	}
	if r.StopStatus != "" {
		v.ResourceStatus = r.StopStatus
	}
	if r.PR != nil {
		p := r.PR
		v.PR = model.PRState{URL: text(p.URL), HeadRevision: text(p.HeadRevision), State: p.State, MergedAt: p.MergedAt, MergeCommit: p.MergeCommit, QueriedAt: &p.CheckedAt, QueryError: text(p.QueryError)}
	}
	if !c.StopRequested && c.Participation == "active" {
		v.Decision = Decision(r)
	}
	return v
}
