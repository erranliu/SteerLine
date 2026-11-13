package app

import (
	"context"
	"errors"
	"slices"

	api "github.com/erranliu/SteerLine/internal/product/api"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func (r *clientReader) taskStartEligibility(ctx context.Context, mode string, task ports.NativeTaskDetail) *api.TaskStartEligibility {
	view := taskStartEligibility(mode, task, slices.Contains(r.executableTypes, task.Type))
	ctx = materialReads(ctx)
	view.EnvironmentReadinessRequired = r.config.environmentPolicy.RequireReadiness()
	if len(view.Blockers) != 0 {
		return view
	}
	if r.readiness != nil {
		readiness, err := r.readiness.projectReadiness(ctx)
		if err != nil {
			view.Blockers = append(view.Blockers, api.TaskStartBlocker{Code: "project_readiness_unverified", Diagnostic: "项目就绪状态待核实：" + err.Error()})
		} else if err = requireProjectReadiness(readiness); err != nil {
			view.Blockers = append(view.Blockers, startBlocker(err))
		}
	}
	if len(view.Blockers) == 0 {
		for _, profile := range r.config.RunnerExecution.Profiles {
			view.ProfileBlockers[profile.ID] = nil
			if err := r.startActor.CheckStartProfile(ctx, profile.ID); err != nil {
				blocker := startBlocker(err)
				view.ProfileBlockers[profile.ID] = &blocker
			}
		}
	}
	return view
}

func taskStartEligibility(mode string, task ports.NativeTaskDetail, executable bool) *api.TaskStartEligibility {
	view := &api.TaskStartEligibility{Action: "start_task", Blockers: []api.TaskStartBlocker{}, ProfileBlockers: map[string]*api.TaskStartBlocker{}}
	candidate := task.Status == "in_progress"
	if candidate {
		view.Action = "start_candidate"
	}
	add := func(code, diagnostic string) {
		view.Blockers = append(view.Blockers, api.TaskStartBlocker{Code: code, Diagnostic: diagnostic})
	}
	switch {
	case task.Delivery != nil:
		add("task_delivered", "任务已交付")
	case task.Status == "discarded":
		add("task_discarded", "任务已放弃")
	case task.Status != "open" && !candidate:
		add("task_not_open", "任务当前无法启动")
	default:
		if !executable {
			add("task_type_not_executable", "当前任务类型不可执行")
		}
		if blocked := taskmodel.StartDispatchError(mode, true, candidate); blocked != nil {
			view.Blockers = append(view.Blockers, startBlocker(blocked))
		}
		if len(task.UnmetPredecessors) > 0 {
			blocker := startBlocker(taskmodel.DependencyClaimError(task.UnmetPredecessors))
			if blocker.Code == "task_not_ready" {
				blocker.Diagnostic = "前置尚未交付"
			}
			view.Blockers = append(view.Blockers, blocker)
		}
		if blocked := taskmodel.CandidateStartError(candidate, task.CurrentMaterialCandidates, task.ObservedMergedCandidate); blocked != nil {
			view.Blockers = append(view.Blockers, startBlocker(blocked))
		}
	}
	return view
}

func startBlocker(err error) api.TaskStartBlocker {
	var domain *taskmodel.Error
	if errors.As(err, &domain) {
		return api.TaskStartBlocker{Code: domain.Code, Diagnostic: domain.Message}
	}
	return api.TaskStartBlocker{Code: "start_eligibility_unverified", Diagnostic: err.Error()}
}
