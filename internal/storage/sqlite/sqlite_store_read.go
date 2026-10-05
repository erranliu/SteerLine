package sqlite

import (
	"context"

	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func (s *SQLiteStore) LookupCommand(ctx context.Context, project, key string) (storageports.CommandRecord, bool, error) {
	return ReadCommandByKey(ctx, s.db, project, key)
}

func (t *sqliteTx) LookupCommand(ctx context.Context, project, key string) (storageports.CommandRecord, bool, error) {
	return ReadCommandByKey(ctx, t.tx, project, key)
}

func (s *SQLiteStore) ServiceCommands(ctx context.Context, project, status string, limit int) ([]storageports.CommandRecord, error) {
	return s.Reader.CommandsFiltered(ctx, project, status, limit, []string{"answer_decision", "retry_run", "mark_stopped", "resolve_command"})
}

func (s *SQLiteStore) Command(ctx context.Context, project, id string) (storageports.CommandRecord, error) {
	v, err := s.Reader.Command(ctx, project, id)
	return v, persistenceError(err)
}

func (s *SQLiteStore) CommandByIdempotency(ctx context.Context, project, key string) (storageports.CommandRecord, error) {
	v, err := s.Reader.CommandByIdempotency(ctx, project, key)
	return v, persistenceError(err)
}
