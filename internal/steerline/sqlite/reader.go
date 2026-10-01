// Package sqlite implements bounded SQLite reads for query ports. The center
// owns database opening, schema creation and atomic business writes.
package sqlite

import (
	"database/sql"

	"github.com/erranliu/SteerLine/internal/steerline/query"
	taskrules "github.com/erranliu/SteerLine/internal/steerline/task"
)

const SchemaVersion = 9

type Reader struct {
	db            *sql.DB
	nativeBinding *taskrules.ProjectTaskAuthority
}

func NewReader(db *sql.DB, binding *taskrules.ProjectTaskAuthority) Reader {
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
