package task

import (
	"time"

	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// CompleteDelivery closes a claimed task only with matching merge evidence.
func CompleteDelivery(record taskmodel.TaskRecord, delivery TaskDeliveryRecord, targetBranch string, at time.Time) (taskmodel.TaskRecord, error) {
	if record.Status != "in_progress" {
		return taskmodel.TaskRecord{}, newError("stale_version", "only an in-progress task can be delivered")
	}
	if delivery.ProjectIdentity != record.ProjectIdentity || delivery.TaskID != record.TaskID {
		return taskmodel.TaskRecord{}, newError("inconsistent_delivery", "delivery belongs to another task")
	}
	if err := ValidateDelivery(delivery, targetBranch); err != nil {
		return taskmodel.TaskRecord{}, err
	}
	next := record
	if err := taskmodel.SetStatus(&next, "closed", false, at); err != nil {
		return taskmodel.TaskRecord{}, err
	}
	return next, nil
}
