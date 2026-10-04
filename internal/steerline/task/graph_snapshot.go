package task

import (
	"github.com/erranliu/SteerLine/internal/task/model"
)

// TaskGraph projects the existing same-read-snapshot graph to task facts. The
// legacy dispatch slot and provider proof remain owned by the #74 boundary.
func (s Snapshot) TaskGraph() model.Snapshot {
	deliveries := make(map[string]model.DeliveryFact, len(s.Deliveries))
	for id, delivery := range s.Deliveries {
		deliveries[id] = model.DeliveryFact{ProjectIdentity: delivery.ProjectIdentity, TaskID: delivery.TaskID}
	}
	return model.Snapshot{Authority: model.GraphAuthority{ProjectIdentity: s.Authority.ProjectIdentity, AuthorityVersion: s.Authority.AuthorityVersion, GraphRevision: s.Authority.GraphRevision}, DatabaseID: s.DatabaseID, DatabaseRevision: s.DatabaseRevision, SampledAt: s.SampledAt, Tasks: s.Tasks, ByID: s.ByID, RepositoryTaskIDs: s.RepositoryTaskIDs, ContractIDs: s.ContractIDs, Edges: s.Edges, Deliveries: deliveries}
}
