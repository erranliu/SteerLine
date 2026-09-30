package steerline

import (
	"context"
	"time"
)

func (s *SQLiteStore) nativeAuthority(ctx context.Context, project string) (ProjectTaskAuthority, error) {
	var v ProjectTaskAuthority
	var paused int
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT authority,authority_version,graph_revision,repository,remote,branch,dispatch_mode,dispatch_paused,created_at,updated_at FROM project_task_authorities WHERE project_identity=?`, project).Scan(&v.Authority, &v.AuthorityVersion, &v.GraphRevision, &v.Repository, &v.Remote, &v.Branch, &v.DispatchMode, &paused, &created, &updated)
	if err != nil {
		return v, err
	}
	v.ProjectIdentity = project
	v.DispatchPaused = paused != 0
	v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return v, nil
}

var _ DomainStore = (*SQLiteStore)(nil)
var _ DomainTx = (*sqliteTx)(nil)
