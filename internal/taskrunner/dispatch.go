package taskrunner

import (
	"context"
	"errors"

	"github.com/erranliu/SteerLine/internal/product/api"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func (s *Service) nextDispatchTask(ready []taskmodel.TaskRecord) (taskmodel.TaskRecord, bool) {
	for _, task := range ready {
		if s.Tasks.CanExecuteType(task.Type) {
			return task, true
		}
	}
	return taskmodel.TaskRecord{}, false
}

// ObserveDispatch projects the persisted mode and current dispatch gates. The
// actor still refreshes admission and the writer rechecks it on reservation.
func (a *Actor) ObserveDispatch(ctx context.Context, checkProject func(context.Context) error) (api.DispatchObservation, error) {
	s := a.options.Tasks
	graph, err := s.Store.TaskGraphSnapshot(ctx, s.Project)
	if err != nil {
		return api.DispatchObservation{}, err
	}
	view := api.DispatchObservation{
		Mode: graph.Authority.DispatchMode, Revision: graph.Authority.AuthorityVersion,
		State: "closed", DefaultProfileID: s.DefaultProfile, DefaultProfileName: s.Profiles[s.DefaultProfile].Name,
	}
	if view.Mode != "auto" {
		return view, nil
	}
	view.State = "waiting"
	switch {
	case graph.ExecutionSlotBusy:
		view.Blocker = &api.DispatchBlocker{Category: "execution", Code: "slot_busy", Diagnostic: "已有执行占用自动派发槽位"}
	default:
		if _, ready := s.nextDispatchTask(graph.ReadySnapshot().Tasks); !ready {
			view.Blocker = &api.DispatchBlocker{Category: "tasks", Code: "no_ready_tasks", Diagnostic: "暂无符合条件的待执行任务"}
		} else if _, err := s.startProfile(s.DefaultProfile); err != nil {
			view.Blocker = dispatchBlocker("profile", "profile_unverified", err)
		} else if err := checkProject(ctx); err != nil {
			view.Blocker = dispatchBlocker("project", "project_readiness_unverified", err)
		} else if err := s.checkExecution(ctx, s.DefaultProfile, false); err != nil {
			view.Blocker = dispatchBlocker("environment", "environment_not_ready", err)
		} else {
			view.State = "ready"
		}
	}
	return view, ctx.Err()
}

func dispatchBlocker(category, code string, err error) *api.DispatchBlocker {
	var domain *taskmodel.Error
	if errors.As(err, &domain) {
		return &api.DispatchBlocker{Category: category, Code: domain.Code, Diagnostic: domain.Message}
	}
	return &api.DispatchBlocker{Category: category, Code: code, Diagnostic: err.Error()}
}
