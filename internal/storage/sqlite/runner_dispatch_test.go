package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/hostcontract"
	"github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskapp "github.com/erranliu/SteerLine/internal/task"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/taskrunner"
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

func TestAutomaticDispatchWaitsForReadinessAndRespectsClose(t *testing.T) {
	for _, scenario := range []string{"dispatch when ready", "close while waiting"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s := openDomainTestStore(t)
			a := taskmodel.ProjectTaskAuthority{ProjectIdentity: "dispatch-readiness", Authority: "native", AuthorityVersion: 1, Repository: "https://example.invalid/Org/Repo.git", Remote: "origin", Branch: "main", DispatchMode: "manual"}
			require(t, s.transactSQLite(ctx, func(tx *sqliteTx) error { return tx.PutProjectTaskAuthority(a) }))
			docs := &runnerTaskDocuments{repository: a.Repository, body: []byte("# Acceptance\nfixed spec and plan\n")}
			sum := sha256.Sum256(docs.body)
			tasks := nativeAuditRecordsWithReference(t, s, a, taskmodel.FileReference{
				Path: "SteerlineFlow/spec/task.spec.md", Commit: strings.Repeat("a", 40), SHA256: hex.EncodeToString(sum[:]),
			})
			service := &taskrunner.Service{
				Project: a.ProjectIdentity, Claimant: "runner-test", StateDir: filepath.Join(t.TempDir(), "runner"), DefaultProfile: "default", Store: s,
				Tasks: taskapp.Service{Reader: s, Documents: docs, Input: taskapp.Input{Project: a.ProjectIdentity, Authority: a.Authority, Remote: a.Remote, Branch: a.Branch, ExecutableTypes: []string{"task"}}},
				Profiles: map[string]taskrunner.Profile{"default": {
					ID: "default", Name: "Dispatch profile", Input: runnerclient.RunInput{Workflow: runnerclient.Workflow{ID: "development", Version: 2}},
				}},
			}
			prepareFixtureServiceEnvironment(t, s, service)
			checkEnvironment := service.CheckExecution
			environmentReady, projectReady := false, false
			service.CheckExecution = func(ctx context.Context, profile string, refresh bool) error {
				if !environmentReady {
					return &taskmodel.Error{Code: "environment_not_ready", Message: "environment is not ready"}
				}
				return checkEnvironment(ctx, profile, refresh)
			}
			checkProject := func(context.Context) error {
				if !projectReady {
					return &taskmodel.Error{Code: "project_not_ready", Message: "project is not ready"}
				}
				return nil
			}
			service.PrepareExecution = func(ctx context.Context, work taskmodel.FixedWork, _ string) (taskmodel.FixedWork, string, error) {
				return work, strings.Repeat("a", 40), checkProject(ctx)
			}
			actor, err := taskrunner.NewActor(taskrunner.ActorOptions{
				Tasks: service, Store: s, Host: &runnerActorHost{}, Repository: hostcontract.Repository{Identity: service.Project},
				Mode: "manual", Heartbeat: time.Second,
				Control: func(context.Context, api.ControlRequest) (api.ControlReceipt, error) {
					return api.ControlReceipt{}, errors.New("unexpected control")
				},
			})
			require(t, err)
			require(t, actor.BeginActor(ctx))
			require(t, actor.Initialize(ctx, cancel))
			tick := func(want int) {
				t.Helper()
				require(t, actor.TickTask(ctx, cancel))
				rows, err := s.RunnerCandidates(ctx, service.Project, "")
				require(t, err)
				if len(rows) != want || want > 0 && (rows[0].TaskID != tasks[0].TaskID || rows[0].Explicit) {
					t.Fatalf("want %d automatic candidates, got %+v", want, rows)
				}
			}
			observe := func(mode, state, code string, revision uint64) {
				t.Helper()
				view, err := actor.ObserveDispatch(ctx, checkProject)
				require(t, err)
				gotCode := ""
				if view.Blocker != nil {
					gotCode = view.Blocker.Code
				}
				if view.Mode != mode || view.State != state || view.Revision != revision || gotCode != code {
					t.Fatalf("unexpected dispatch observation: %+v, blocker %s", view, gotCode)
				}
			}
			observe("manual", "closed", "", 1)
			on, err := s.SetRunnerDispatch(ctx, ports.RunnerDispatch{Project: service.Project, RequestID: "on", ExpectedAuthorityVersion: 1, Mode: "auto", RequestedBy: "operator"})
			require(t, err)
			observe("auto", "waiting", "project_not_ready", on.AuthorityVersion)
			environmentReady = true
			tick(0)
			projectReady = true
			environmentReady = false
			observe("auto", "waiting", "environment_not_ready", on.AuthorityVersion)
			tick(0)

			if scenario == "close while waiting" {
				off, err := s.SetRunnerDispatch(ctx, ports.RunnerDispatch{Project: service.Project, RequestID: "off", ExpectedAuthorityVersion: on.AuthorityVersion, Mode: "manual", RequestedBy: "operator"})
				require(t, err)
				environmentReady = true
				observe("manual", "closed", "", off.AuthorityVersion)
				tick(0)
			} else {
				environmentReady = true
				observe("auto", "ready", "", on.AuthorityVersion)
				tick(1)
				observe("auto", "waiting", "slot_busy", on.AuthorityVersion)
			}
		})
	}
}
