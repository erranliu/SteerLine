package ports

import (
	"time"

	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// TaskGraphSnapshot is the single sampled read value shared by task commands
// and product graph queries. It contains no query filters, cursor, or page.
type TaskGraphSnapshot struct {
	Authority         taskmodel.ProjectTaskAuthority
	DatabaseID        string
	DatabaseRevision  uint64
	SampledAt         time.Time
	Tasks             []taskmodel.TaskRecord
	ByID              map[string]taskmodel.TaskRecord
	RepositoryTaskIDs map[string]bool
	ContractIDs       map[string]bool
	Edges             []taskmodel.TaskDependencyRecord
	Deliveries        map[string]taskmodel.TaskDeliveryRecord
	ExecutionSlotBusy bool
}

// TaskGraph projects the sampled records to the task domain's dependency view.
func (s TaskGraphSnapshot) TaskGraph() taskmodel.Snapshot {
	deliveries := make(map[string]taskmodel.DeliveryFact, len(s.Deliveries))
	for id, delivery := range s.Deliveries {
		deliveries[id] = taskmodel.DeliveryFact{ProjectIdentity: delivery.ProjectIdentity, TaskID: delivery.TaskID}
	}
	return taskmodel.Snapshot{Authority: taskmodel.GraphAuthority{ProjectIdentity: s.Authority.ProjectIdentity, AuthorityVersion: s.Authority.AuthorityVersion, GraphRevision: s.Authority.GraphRevision}, DatabaseID: s.DatabaseID, DatabaseRevision: s.DatabaseRevision, SampledAt: s.SampledAt, Tasks: s.Tasks, ByID: s.ByID, RepositoryTaskIDs: s.RepositoryTaskIDs, ContractIDs: s.ContractIDs, Edges: s.Edges, Deliveries: deliveries}
}

// ReadySnapshot applies task-domain eligibility to the same sampled facts.
// It contains no product DTO, transport protocol or cursor.
func (s TaskGraphSnapshot) ReadySnapshot() NativeReadyTaskSnapshot {
	return NativeReadyTaskSnapshot{ProjectID: s.Authority.ProjectIdentity, AuthorityVersion: s.Authority.AuthorityVersion, GraphRevision: s.Authority.GraphRevision, DatabaseID: s.DatabaseID, DatabaseRevision: s.DatabaseRevision, SampledAt: s.SampledAt, Tasks: taskmodel.ReadyTasks(s.TaskGraph())}
}
