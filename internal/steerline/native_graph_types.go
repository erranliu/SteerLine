package steerline

import "github.com/erranliu/SteerLine/internal/steerline/taskview"

const nativeTaskGraphProtocol = taskview.Protocol

type TaskGraphNode = taskview.Node
type TaskGraphEdge = taskview.Edge
type TaskGraphFilter = taskview.Filter
type TaskGraphBoundaryEdge = taskview.BoundaryEdge
type TaskGraphBoundary = taskview.Boundary
type TaskGraphPage = taskview.Page
