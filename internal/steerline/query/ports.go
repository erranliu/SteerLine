// Package query owns read-only application queries. It depends on data
// contracts and pure task rules; storage implementations supply the read ports.
package query

import (
	"context"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	taskview "github.com/erranliu/SteerLine/internal/steerline/taskview"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

type WorkflowReader interface {
	Query(context.Context, string) (contracts.DomainSnapshot, error)
}

type EventReader interface {
	Events(context.Context, string, string, string, int64, int, int) ([]storageports.DomainEvent, error)
	EventPayload(context.Context, string, int64, int64, int) ([]byte, int64, string, error)
}

type TaskReader interface {
	NativeTaskPage(context.Context, string, string, string, uint64, int, int) (contracts.NativeTaskPage, error)
	TaskGraphSnapshot(context.Context, string) (taskview.Snapshot, error)
}

type ProjectReader interface {
	ReadProjectView(context.Context, string, string, int) (contracts.ProjectView, error)
}

// Store composes the read capabilities exposed by the product query facade.
// No connection ownership, write transaction or command mutation is available.
type Store interface {
	WorkflowReader
	EventReader
	TaskReader
	ProjectReader
}

// NewService wires one product reader into its separate consumer capabilities.
func NewService(store Store, project string) Service {
	return Service{Project: project, Workflows: store, Events: store, Tasks: store, Projects: store}
}
