package sqlite

import (
	"context"

	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func (s *SQLiteStore) BindAuthority(v taskmodel.ProjectTaskAuthority) {
	s.Reader = NewReader(s.db, &v)
}

func (s *SQLiteStore) ValidateStoredFormat(ctx context.Context) error {
	if err := validateRetiredWholeRunResultCommands(ctx, s.db); err != nil {
		return err
	}
	return validateNativeSQLiteRuntimeProjection(ctx, s.db)
}
