package steerline

import (
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func nativeTaskRecord(tasks []taskmodel.TaskRecord, taskID string) *taskmodel.TaskRecord {
	for i := range tasks {
		if tasks[i].TaskID == taskID {
			return &tasks[i]
		}
	}
	return nil
}
