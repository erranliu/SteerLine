package steerline

import (
	"context"
	"time"
)

func (s *SQLiteStore) NativeAuthority(ctx context.Context, project string) (ProjectTaskAuthority, error) {
	return readNativeAuthority(ctx, s.db, project)
}

func (tx *sqliteTx) NativeAuthority(ctx context.Context, project string) (ProjectTaskAuthority, error) {
	return readNativeAuthority(ctx, tx.tx, project)
}

func readNativeAuthority(ctx context.Context, q commandRowQuerier, project string) (ProjectTaskAuthority, error) {
	var v ProjectTaskAuthority
	var paused int
	var created, updated string
	err := q.QueryRowContext(ctx, `SELECT authority,authority_version,graph_revision,repository,remote,branch,dispatch_mode,dispatch_paused,created_at,updated_at FROM project_task_authorities WHERE project_identity=?`, project).Scan(&v.Authority, &v.AuthorityVersion, &v.GraphRevision, &v.Repository, &v.Remote, &v.Branch, &v.DispatchMode, &paused, &created, &updated)
	if err != nil {
		return v, err
	}
	v.ProjectIdentity = project
	v.DispatchPaused = paused != 0
	v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return v, nil
}
