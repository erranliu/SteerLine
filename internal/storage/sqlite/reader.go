// Package sqlite owns connections, current schema, bounded reads and atomic
// declarative writes. The application injects its ports into center consumers.
package sqlite

import (
	"database/sql"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

const SchemaVersion = 18

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
