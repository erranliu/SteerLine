// Package sqlite owns connections, current schema, bounded reads and atomic
// declarative writes. The application injects its ports into center consumers.
package sqlite

import (
	"database/sql"

	"github.com/erranliu/SteerLine/internal/steerline/query"
	tasklegacy "github.com/erranliu/SteerLine/internal/steerline/task"
)

const SchemaVersion = 13

type Reader struct {
	db            *sql.DB
	nativeBinding *tasklegacy.ProjectTaskAuthority
}

func NewReader(db *sql.DB, binding *tasklegacy.ProjectTaskAuthority) Reader {
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
