package model

import (
	"time"
)

// DeliveryFact is the task-level conclusion of a committed delivery. Execution
// retains its selected workflow, session and provider proof separately.
type DeliveryFact struct {
	ProjectIdentity string
	TaskID          string
}

// Snapshot contains only the task graph and committed task delivery facts.
type Snapshot struct {
	Authority         GraphAuthority
	DatabaseID        string
	DatabaseRevision  uint64
	SampledAt         time.Time
	Tasks             []TaskRecord
	ByID              map[string]TaskRecord
	RepositoryTaskIDs map[string]bool
	ContractIDs       map[string]bool
	Edges             []TaskDependencyRecord
	Deliveries        map[string]DeliveryFact
}

// GraphAuthority contains the task identity and graph versions from one read.
type GraphAuthority struct {
	ProjectIdentity                 string
	AuthorityVersion, GraphRevision uint64
}
