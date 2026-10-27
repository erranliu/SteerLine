// Package sqlite owns connections, current schema, bounded reads and atomic
// declarative writes. The application injects its ports into center consumers.
package sqlite

import (
	"database/sql"

	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

const SchemaVersion = 32

type Reader struct {
	db            *sql.DB
	nativeBinding *taskmodel.ProjectTaskAuthority
}

func NewReader(db *sql.DB, binding *taskmodel.ProjectTaskAuthority) Reader {
	if binding != nil {
		fixed := *binding
		binding = &fixed
	}
	return Reader{db: db, nativeBinding: binding}
}
