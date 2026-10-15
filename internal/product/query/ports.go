// Package query owns read-only application queries. It depends on data
// contracts and pure task rules; storage implementations supply the read ports.
package query

import storageports "github.com/erranliu/SteerLine/internal/storage/ports"

// NewService wires one product reader into its separate consumer capabilities.
func NewService(store storageports.ProductReader, project string) Service {
	return Service{Project: project, Events: store, Tasks: store, Projects: store, Activity: store}
}
