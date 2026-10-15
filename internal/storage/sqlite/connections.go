package sqlite

import (
	"context"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func (s *SQLiteStore) BindAuthority(v executionmodel.ProjectTaskAuthority) {
	s.Reader = NewReader(s.db, &v)
}

func (s *SQLiteStore) ValidateStoredFormat(ctx context.Context) error {
	if err := validateRetiredWholeRunResultCommands(ctx, s.db); err != nil {
		return err
	}
	return validateNativeSQLiteRuntimeProjection(ctx, s.db)
}
