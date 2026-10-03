package contracts

import (
	"errors"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
)

func MarkWorkerDeliveryCommitted(task TaskRecord, workflow *WorkflowRunRecord, delivery TaskDeliveryRecord) error {
	if workflow == nil {
		return errors.New("completed worker flow requires committed matching delivery")
	}
	state := workerflow.State{
		WorkflowRunID: workflow.WorkflowRunID, TaskID: workflow.TaskID, WorkflowVersion: workflow.WorkflowVersion,
		Phase: workflow.Phase, PRURL: workflow.PRURL, PRState: workflow.PRState, WorkflowUpdatedAt: workflow.UpdatedAt,
	}
	next, err := workerflow.Complete(state, task.TaskID, task.Status, workerflow.Delivery{
		TaskID: delivery.TaskID, WorkflowRunID: delivery.SelectedWorkflowRunID, PRURL: delivery.PRURL,
		MergeCommit: delivery.MergeCommit, MergedAt: delivery.MergedAt,
	}, time.Now().UTC())
	if err == nil {
		workflow.Phase, workflow.PRState, workflow.UpdatedAt = next.Phase, next.PRState, next.WorkflowUpdatedAt
	}
	return err
}
