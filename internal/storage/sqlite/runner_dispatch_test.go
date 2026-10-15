package sqlite

import (
	"context"
	"testing"

	ports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func TestRunnerDispatchReceiptAndAuthorityCommitTogether(t *testing.T) {
	ctx := context.Background()
	s, project, _ := nativeAuditStore(t)
	paused := false
	in := ports.RunnerDispatch{Project: project, RequestID: "runner-dispatch", ExpectedAuthorityVersion: 1, Paused: &paused, RequestedBy: "operator"}
	_, err := s.db.ExecContext(ctx, `CREATE TRIGGER fail_runner_dispatch BEFORE UPDATE ON commands WHEN NEW.kind='runner_dispatch' BEGIN SELECT RAISE(ABORT,'receipt fault'); END`)
	require(t, err)
	if _, err := s.SetRunnerDispatch(ctx, in); err == nil {
		t.Fatal("receipt fault did not abort authority change")
	}
	authority, err := s.NativeAuthority(ctx, project)
	require(t, err)
	if authority.AuthorityVersion != 1 || !authority.DispatchPaused {
		t.Fatal("authority escaped rollback")
	}
	if _, found, err := s.LookupCommand(ctx, project, in.RequestID); err != nil || found {
		t.Fatal("failed dispatch left a command", err)
	}
	_, err = s.db.ExecContext(ctx, `DROP TRIGGER fail_runner_dispatch`)
	require(t, err)
	first, err := s.SetRunnerDispatch(ctx, in)
	require(t, err)
	second, err := s.SetRunnerDispatch(ctx, in)
	require(t, err)
	if first.AuthorityVersion != 2 || first.DispatchPaused || second.AuthorityVersion != 2 {
		t.Fatal("dispatch replay changed authority")
	}
	paused = true
	if _, err := s.SetRunnerDispatch(ctx, in); err == nil {
		t.Fatal("changed original intent reused receipt")
	}
}
