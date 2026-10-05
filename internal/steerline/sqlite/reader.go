// Package sqlite owns connections, current schema, bounded reads and atomic
// declarative writes. The application injects its ports into center consumers.
package sqlite

import (
	"database/sql"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/product/query"
)

const SchemaVersion = 14

type Reader struct {
	db            *sql.DB
	nativeBinding *executionmodel.ProjectTaskAuthority
}

func NewReader(db *sql.DB, binding *executionmodel.ProjectTaskAuthority) Reader {
	if binding != nil {
		fixed := *binding
		binding = &fixed
	}
	return Reader{db: db, nativeBinding: binding}
}

var _ query.WorkflowReader = Reader{}

var _ query.EventReader = Reader{}

var _ query.TaskReader = Reader{}

var _ query.ProjectReader = Reader{}
