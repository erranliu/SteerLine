package steerline

import (
	"context"
	"database/sql"

	"github.com/erranliu/SteerLine/internal/steerline/sqlite"
)

func (s *SQLiteStore) reader() sqlite.Reader { return sqlite.NewReader(s.db, s.nativeBinding) }
func (s *SQLiteStore) LoadProjection(ctx context.Context, project string) ([]byte, uint64, error) {
	return s.reader().LoadProjection(ctx, project)
}
func (s *SQLiteStore) DatabaseVersion(ctx context.Context) (string, uint64, error) {
	return s.reader().DatabaseVersion(ctx)
}
func (s *SQLiteStore) Query(ctx context.Context, project string) (DomainSnapshot, error) {
	return s.reader().Query(ctx, project)
}

type queryContext = sqlite.QueryContext
type commandRowQuerier = sqlite.CommandRowQuerier

func (s *SQLiteStore) LookupCommand(ctx context.Context, project, key string) (CommandRecord, bool, error) {
	return sqlite.ReadCommandByKey(ctx, s.db, project, key)
}

func (t *sqliteTx) LookupCommand(ctx context.Context, project, key string) (CommandRecord, bool, error) {
	return sqlite.ReadCommandByKey(ctx, t.tx, project, key)
}

func querySnapshot(ctx context.Context, db queryContext, project string) (DomainSnapshot, error) {
	return sqlite.QuerySnapshot(ctx, db, project)
}
func (s *SQLiteStore) Events(ctx context.Context, project, execution, attempt string, after int64, limit, budget int) ([]DomainEvent, error) {
	return s.reader().Events(ctx, project, execution, attempt, after, limit, budget)
}
func (s *SQLiteStore) EventPayload(ctx context.Context, project string, id, offset int64, limit int) ([]byte, int64, string, error) {
	return s.reader().EventPayload(ctx, project, id, offset, limit)
}
func (s *SQLiteStore) Commands(ctx context.Context, project, status string, limit int) ([]CommandRecord, error) {
	return s.reader().Commands(ctx, project, status, limit)
}
func (s *SQLiteStore) ServiceCommands(ctx context.Context, project, status string, limit int) ([]CommandRecord, error) {
	return s.reader().CommandsFiltered(ctx, project, status, limit, []string{"answer_decision", "retry_run", "mark_stopped", "resolve_command"})
}
func (s *SQLiteStore) Command(ctx context.Context, project, id string) (CommandRecord, error) {
	v, err := s.reader().Command(ctx, project, id)
	return v, persistenceError(err)
}
func (s *SQLiteStore) CommandByIdempotency(ctx context.Context, project, key string) (CommandRecord, error) {
	v, err := s.reader().CommandByIdempotency(ctx, project, key)
	return v, persistenceError(err)
}
func (s *SQLiteStore) NativeTaskPage(ctx context.Context, project, id, databaseID string, revision uint64, offset, limit int) (NativeTaskPage, error) {
	return s.reader().NativeTaskPage(ctx, project, id, databaseID, revision, offset, limit)
}
func (s *SQLiteStore) nativeTaskPageTx(ctx context.Context, tx *sql.Tx, project, id, databaseID string, revision uint64, offset, limit int) (NativeTaskPage, error) {
	return s.reader().NativeTaskPageTx(ctx, tx, project, id, databaseID, revision, offset, limit)
}
