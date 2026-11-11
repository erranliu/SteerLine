package sqlite

import (
	"context"
	"testing"

	ports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func TestRunnerDispatchReceiptAndAuthorityCommitTogether(t *testing.T) {
	ctx := context.Background()
	s, project, _ := nativeAuditStore(t)
	in := ports.RunnerDispatch{Project: project, RequestID: "runner-dispatch", ExpectedAuthorityVersion: 1, Mode: "auto", RequestedBy: "operator"}
	_, err := s.db.ExecContext(ctx, `CREATE TRIGGER fail_runner_dispatch BEFORE UPDATE ON commands WHEN NEW.kind='runner_dispatch' BEGIN SELECT RAISE(ABORT,'receipt fault'); END`)
	require(t, err)
	if _, err := s.SetRunnerDispatch(ctx, in); err == nil {
		t.Fatal("receipt fault did not abort authority change")
	}
	authority, err := s.NativeAuthority(ctx, project)
	require(t, err)
	if authority.AuthorityVersion != 1 || authority.DispatchMode != "manual" {
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
	if first.AuthorityVersion != 2 || first.DispatchMode != "auto" || second.AuthorityVersion != 2 || second.DispatchMode != "auto" {
		t.Fatal("dispatch replay changed authority")
	}
	in.Mode = "manual"
	if _, err := s.SetRunnerDispatch(ctx, in); err == nil {
		t.Fatal("changed original intent reused receipt")
	}
	in.RequestID = "runner-dispatch-off"
	in.ExpectedAuthorityVersion = first.AuthorityVersion
	disabled, err := s.SetRunnerDispatch(ctx, in)
	require(t, err)
	authority, err = s.NativeAuthority(ctx, project)
	require(t, err)
	if disabled.AuthorityVersion != 3 || authority.AuthorityVersion != 3 || authority.DispatchMode != "manual" {
		t.Fatal("turning automatic dispatch off did not persist manual mode")
	}
}
