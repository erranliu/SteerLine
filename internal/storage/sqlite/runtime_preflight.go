package sqlite

import (
	"context"
	"database/sql"

	"github.com/erranliu/SteerLine/internal/hostcontract"
)

func validateNativeSQLiteRuntimeProjection(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT payload FROM projection`)
	if err != nil {
		return schemaFailure(err, "cannot inspect the saved runtime projection without mutation")
	}
	defer rows.Close()
	for rows.Next() {
		var payload []byte
		if err = rows.Scan(&payload); err != nil {
			return schemaFailure(err, "cannot read the saved runtime projection without mutation")
		}
		var checkpoint hostcontract.ServiceCheckpoint
		if err = decode(payload, &checkpoint); err != nil || checkpoint.SchemaVersion != hostcontract.ServiceCheckpointSchema || checkpoint.Repository.Identity == "" {
			return nativeErr("unsupported_schema", "unsupported service checkpoint; preserve the database")
		}

	}
	if err = rows.Err(); err != nil {
		return schemaFailure(err, "cannot inspect the saved runtime projection without mutation")
	}
	return nil
}
