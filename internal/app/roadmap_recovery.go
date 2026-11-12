package app

import (
	"context"
	"encoding/json"
	"errors"

	roadmapservice "github.com/erranliu/SteerLine/internal/roadmap"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

func roadmapContent(r roadmap.Record) runnerbinding.Content {
	return runnerbinding.Content{StateDir: r.Runner.StateDir, Request: r.Request, RunID: r.RunID, InputDigest: r.InputDigest, Project: r.Project, ObjectKey: "steerline_roadmap_parse", ObjectID: r.ID, Unsubmitted: r.Submission == "pending", Rejected: r.State == "rejected" && r.RunID == ""}
}

func preparePlanningStops(ctx context.Context, c *Config, store *sqlite.SQLiteStore, cli *runnerclient.Client) ([]runnercontract.PendingStop, error) {
	tasks, err := store.PlanningTasks(ctx, c.Identity())
	if err != nil {
		return nil, err
	}
	p := planningPort{roadmapPort: &roadmapPort{store: store}}
	var pending []runnercontract.PendingStop
	for _, task := range tasks {
		if (!task.StopRequested && task.Merged == "") || (task.Run == nil && task.Operation == nil) || task.Run != nil && task.Run.StopStatus == "stopped" || !runnerbinding.SameBuild(task.Build, cli.BuildPin()) {
			continue
		}
		var runID, digest string
		if task.Run != nil {
			runID, digest = task.Run.RunID, task.Run.InputDigest
		}
		view, cold, err := inspectOriginalBeforeStart(ctx, *cli, task.Build.StateDir, runID, digest, task.Submission)
		if err != nil {
			var failure *runnerclient.Failure
			unaccepted := submitNotAccepted(view, cold, task.Submission.RequestID, err)
			if runID == "" && (unaccepted || errors.As(err, &failure) && failure.Code == "request_not_found") {
				if unaccepted {
					task.Operation, task.Status = nil, "closed"
					task.ModelSlot = false
					if err := p.save(ctx, &task); err != nil {
						return nil, err
					}
				}
				continue
			}
			return nil, err
		}
		if err := verifyPlanningRun(&task, view.Run); err != nil {
			return nil, err
		}
		if !cold || !planningStartupCanStop(&task, view.Run) {
			continue
		}
		if task.Operation != nil {
			prior, offline, err := cli.InspectBeforeStart(ctx, "request-id", task.Operation.Request.RequestID)
			if err == nil {
				if err := verifyPlanningRun(&task, prior.Run); err != nil {
					return nil, err
				}
				if prior.Command == nil || prior.Command.Command != "stop" {
					return nil, errors.New("原规划停止请求身份无法核实")
				}
				continue
			}
			var failure *runnerclient.Failure
			if !offline || !errors.As(err, &failure) || failure.Code != "request_not_found" {
				return nil, err
			}
		}
		task.Run = view.Run
		if preparePlanningStartupRequest(&task) {
			if err := p.save(ctx, &task); err != nil {
				return nil, err
			}
		}
		pending = append(pending, runnercontract.PendingStop{RunID: task.Run.RunID, Request: *task.Operation.Request})
	}
	return pending, nil
}

func planningStartupCanStop(task *roadmap.PlanningTask, run *runnercontract.Run) bool {
	return run.WorkspaceWrite != nil && run.WorkspaceWrite.Owner == "runner" && run.Controls.CanStop && run.StopStatus == "" &&
		(task.Operation == nil || task.Operation.Kind == "close" && task.Operation.Command == "stop" && task.Operation.Request != nil)
}

func preparePlanningStartupRequest(task *roadmap.PlanningTask) bool {
	if op := task.Operation; op != nil {
		var previous struct {
			Revision uint64 `json:"expected_revision"`
		}
		if json.Unmarshal(op.Request.Input, &previous) == nil && previous.Revision == task.Run.Revision {
			return false
		}
	}
	task.Operation = &roadmap.PlanningOperation{Kind: "close"}
	planControl(task.Operation, "stop", task.Run.Revision, nil)
	return true
}

func verifyPlanningLifecycle(ctx context.Context, tasks []roadmap.PlanningTask, proof runnerclient.StoppedVerification, restart *runnercontract.BuildPin) error {
	for _, task := range tasks {
		if task.Run == nil && task.Operation == nil {
			continue
		}
		if restart != nil {
			if planningLeaseReturnedAndStopped(task.Run) && task.Operation == nil {
				continue
			}
			if err := verifyPlanningRestart(ctx, &task, *restart); err != nil {
				return err
			}
			continue
		}
		if !runnerbinding.SamePath(task.Build.StateDir, proof.StateDir) || !planningLeaseReturnedAndStopped(task.Run) {
			return errors.New("original Roadmap planning lease return or session stop is unverified")
		}
		settled := false
		for _, run := range proof.Runs {
			if run.RunID == task.Run.RunID && run.InputDigest != "" && run.InputDigest == task.Run.InputDigest && run.ResourcesSettled {
				settled = true
				break
			}
		}
		if !settled {
			return errors.New("original Roadmap planning resource settlement is unverified")
		}
	}
	return nil
}

func planningLeaseReturnedAndStopped(run *runnercontract.Run) bool {
	return run != nil && run.RunID != "" && run.StopStatus == "stopped" &&
		run.WorkspaceWrite != nil && run.WorkspaceWrite.Owner == "runner" && run.WorkspaceWrite.LeaseID == ""
}

func verifyPlanningRestart(ctx context.Context, task *roadmap.PlanningTask, pin runnercontract.BuildPin) error {
	if !runnerbinding.SameBuild(task.Build, pin) {
		return errors.New("Roadmap planning requires the original Runner build for restart")
	}
	cli := runnerclient.Client{Executable: pin.Executable, StateDir: pin.StateDir, SHA256: pin.SHA256, BuildVersion: pin.Version, ResourceDigest: pin.ResourceDigest}
	var runID, digest string
	if task.Run != nil {
		runID, digest = task.Run.RunID, task.Run.InputDigest
	}
	view, cold, err := inspectOriginalBeforeStart(ctx, cli, task.Build.StateDir, runID, digest, task.Submission)
	if err != nil {
		if runID == "" && submitNotAccepted(view, cold, task.Submission.RequestID, err) {
			return nil
		}
		return err
	}
	return verifyPlanningRun(task, view.Run)
}

func prepareRoadmapStops(ctx context.Context, c *Config, store *sqlite.SQLiteStore, cli *runnerclient.Client) ([]runnercontract.PendingStop, error) {
	rows, e := store.Roadmaps(ctx, c.Identity())
	if e != nil {
		return nil, e
	}
	pending := []runnercontract.PendingStop{}
	reconcile := &roadmapservice.Service{Project: c.Identity(), Store: store}
	for _, parent := range rows {
		for _, r := range parent.Executions() {
			if c.runtimeGenerations != nil {
				c.runtimeGenerations.UpdateContent(c.Identity(), "roadmap:"+func() string {
					if r.GenerationID != "" {
						return r.GenerationID
					}
					return r.ID
				}(), r.CreatedAt, r.Submission != "pending", r.Settled())
			}
			if r.Submission == "pending" || r.State == "rejected" && r.RunID == "" || r.ObservationComplete() {
				continue
			}
			if !r.Cancelled || r.Projection != nil && r.Projection.StopStatus == "stopped" {
				continue
			}
			view, cold, err := inspectOriginalBeforeStart(ctx, *cli, r.Runner.StateDir, r.RunID, r.InputDigest, r.Request)
			if err != nil {
				if r.RunID == "" && submitNotAccepted(view, cold, r.Request.RequestID, err) {
					r.State = "rejected"
					if _, err := reconcile.SaveExecution(ctx, r); err != nil {
						return nil, err
					}
					continue
				}
				return nil, err
			}
			run := view.Run
			if err := runnerbinding.ValidateContentCorrelation(roadmapContent(r), run); err != nil {
				return nil, err
			}
			if r.StopRequest == nil || r.StopState == "rejected" {
				r.StopRequest = runnerbinding.ContentStop(roadmapservice.Token(), run.Revision)
				r.StopState = "unknown"
				if _, e = reconcile.SaveExecution(ctx, r); e != nil {
					return nil, e
				}
			}
			pending = append(pending, runnercontract.PendingStop{RunID: run.RunID, Request: *r.StopRequest})
		}
	}
	return pending, nil
}
