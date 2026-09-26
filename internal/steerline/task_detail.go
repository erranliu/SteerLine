package steerline

import (
	"context"
	"fmt"
	"time"
)

// TaskDetail is the read-only, human-facing detail shown by the run viewer.
type TaskDetail struct {
	ID          string
	Title       string
	Type        string
	Status      string
	Assignee    string
	Priority    *int
	Description string
	SpecID      string
	Labels      []string
}

// LoadTaskDetail reads the native task authority. It never changes task state.
func LoadTaskDetail(ctx context.Context, c Config, id string) (TaskDetail, error) {
	if err := requireNativeTaskAuthority(c); err != nil {
		return TaskDetail{}, err
	}
	if err := ValidateNativeTaskID(id); err != nil {
		return TaskDetail{}, err
	}
	store, err := OpenNativeTaskProject(ctx, &c)
	if err != nil {
		return TaskDetail{}, err
	}
	defer store.Close()
	page, err := (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, id)
	if err != nil {
		return TaskDetail{}, err
	}
	if len(page.Tasks) != 1 {
		return TaskDetail{}, nativeErr("task_not_found", "native task detail query returned no unique task")
	}
	task := page.Tasks[0]
	priority := task.TaskRecord.Priority
	return TaskDetail{
		ID: task.TaskID, Title: task.Title, Type: task.Type, Status: task.Status,
		Assignee: task.Claimant, Priority: &priority, SpecID: task.Contract.SpecRef.Path,
		Description: fmt.Sprintf("Spec: %s @ %s\nPlan: %s @ %s\nSampled: %s", task.Contract.SpecRef.Path, task.Contract.SpecRef.Commit, task.Contract.PlanRef.Path, task.Contract.PlanRef.Commit, page.SampledAt.Format(time.RFC3339Nano)),
	}, nil
}
