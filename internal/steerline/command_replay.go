package steerline

import (
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// This abort-only marker is consumed by execution command transactions when
// replaying an existing receipt. The execution-owned commands currently use
// Status from this value for cleanup/discard/dispatch replay.
type taskBatchReplay struct{ Receipt taskmodel.TaskBatchReceipt }

func (taskBatchReplay) Error() string { return "command already applied" }
