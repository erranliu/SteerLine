package taskview

import (
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	model "github.com/erranliu/SteerLine/internal/task/model"
)

type Snapshot struct {
	Authority         executionmodel.ProjectTaskAuthority
	DatabaseID        string
	DatabaseRevision  uint64
	SampledAt         time.Time
	Tasks             []model.TaskRecord
	ByID              map[string]model.TaskRecord
	RepositoryTaskIDs map[string]bool
	ContractIDs       map[string]bool
	Edges             []model.TaskDependencyRecord
	Deliveries        map[string]executionmodel.TaskDeliveryRecord
	ExecutionSlotBusy bool
}

// TaskGraph projects the existing same-read-snapshot graph to task facts. The
// dispatch slot and provider proof stay in this read projection, outside task.
func (s Snapshot) TaskGraph() model.Snapshot {
	deliveries := make(map[string]model.DeliveryFact, len(s.Deliveries))
	for id, delivery := range s.Deliveries {
		deliveries[id] = model.DeliveryFact{ProjectIdentity: delivery.ProjectIdentity, TaskID: delivery.TaskID}
	}
	return model.Snapshot{Authority: model.GraphAuthority{ProjectIdentity: s.Authority.ProjectIdentity, AuthorityVersion: s.Authority.AuthorityVersion, GraphRevision: s.Authority.GraphRevision}, DatabaseID: s.DatabaseID, DatabaseRevision: s.DatabaseRevision, SampledAt: s.SampledAt, Tasks: s.Tasks, ByID: s.ByID, RepositoryTaskIDs: s.RepositoryTaskIDs, ContractIDs: s.ContractIDs, Edges: s.Edges, Deliveries: deliveries}
}
