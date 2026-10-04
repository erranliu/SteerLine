package task

import (
	"time"

	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type Snapshot struct {
	Authority         ProjectTaskAuthority
	DatabaseID        string
	DatabaseRevision  uint64
	SampledAt         time.Time
	Tasks             []taskmodel.TaskRecord
	ByID              map[string]taskmodel.TaskRecord
	RepositoryTaskIDs map[string]bool
	ContractIDs       map[string]bool
	Edges             []taskmodel.TaskDependencyRecord
	Deliveries        map[string]TaskDeliveryRecord
	ExecutionSlotBusy bool
}
