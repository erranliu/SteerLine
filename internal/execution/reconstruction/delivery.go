package reconstruction

import (
	"errors"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func MarkWorkerDeliveryCommitted(task taskmodel.TaskRecord, workflow *executionmodel.WorkflowRunRecord, delivery executionmodel.TaskDeliveryRecord) error {
	if workflow == nil {
		return errors.New("completed worker flow requires committed matching delivery")
	}
	state := executionmodel.State{
		WorkflowRunID: workflow.WorkflowRunID, TaskID: workflow.TaskID, WorkflowVersion: workflow.WorkflowVersion,
		Status: workflow.Status, PRURL: workflow.PRURL, PRState: workflow.PRState, WorkflowUpdatedAt: workflow.UpdatedAt,
	}
	next, err := executionmodel.Complete(state, task.TaskID, task.Status, executionmodel.Delivery{
		TaskID: delivery.TaskID, WorkflowRunID: delivery.SelectedWorkflowRunID, PRURL: delivery.PRURL,
		MergeCommit: delivery.MergeCommit, MergedAt: delivery.MergedAt,
	}, time.Now().UTC())
	if err == nil {
		workflow.Status, workflow.PRState, workflow.UpdatedAt = next.Status, next.PRState, next.WorkflowUpdatedAt
		workflow.StepID = executionmodel.StepDelivered
	}
	return err
}
