package execution

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	query "github.com/erranliu/SteerLine/internal/product/query"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type ps3ResourceGuard struct {
	executionbackend.ManagedProcessHost
	destructive atomic.Int64
}

func (g *ps3ResourceGuard) check(args []string) error {
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "worktree remove") || strings.Contains(joined, "branch -D") || strings.Contains(joined, "pr close") {
		g.destructive.Add(1)
		return fmt.Errorf("product attempted destructive cleanup: %s", joined)
	}
	return nil
}
func (g *ps3ResourceGuard) StartManaged(cmd *exec.Cmd) (executionbackend.StopHandle, error) {
	if err := g.check(cmd.Args); err != nil {
		return nil, err
	}
	return g.ManagedProcessHost.StartManaged(cmd)
}
func (g *ps3ResourceGuard) OutputCommand(ctx context.Context, cmd executionbackend.LaunchCommand) ([]byte, error) {
	if err := g.check(cmd.Args); err != nil {
		return nil, err
	}
	return g.ManagedProcessHost.OutputCommand(ctx, cmd)
}

func TestPS3DiscardPreservesTwoCandidateResourcesAndPRs(t *testing.T) {
	f := newMCFixture(t)
	receipt, _ := f.start(t, "candidate-b")
	second := f.s.find(receipt.WorkflowRunID)
	paths := map[string]string{}
	for i, run := range []string{f.first.RunID, second.RunID} {
		candidate := f.s.find(run)
		mcStartTurn(t, f.s, candidate)
		mcFinishTurn(t, f.s, candidate)
		f.bind(t, candidate, 81+i, "open")
		paths[run] = f.s.executionRecord(candidate).Workspace.Worktree
		if _, err := os.Stat(paths[run]); err != nil {
			t.Fatal(err)
		}
	}
	guard := &ps3ResourceGuard{ManagedProcessHost: f.s.runtime.ProcessHost}
	f.s.runtime.ProcessHost, f.commands.runtime.ProcessHost = guard, guard
	discarded, err := f.commands.DiscardNativeTask(context.Background(), f.c, "8", newID(), 2, "retain both resources", "operator")
	require(t, err)
	if discarded.Status != "discarded" {
		t.Fatalf("discard: %+v", discarded)
	}
	for _, item := range discarded.StopPending {
		if !strings.HasPrefix(item, "execution_stop_manual:") {
			t.Fatalf("destructive pending item: %s", item)
		}
	}
	require(t, f.s.refreshNativeSnapshot(context.Background()))
	require(t, f.s.reconcileDiscardedExecutionStops(context.Background()))
	for id, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("resource %s removed: %v", id, err)
		}
		branch := f.s.executionRecord(f.s.find(id)).Workspace.Branch
		if _, err := exec.Command("git", "-C", f.c.Repository, "show-ref", "--verify", "refs/heads/"+branch).CombinedOutput(); err != nil {
			t.Fatal(err)
		}
	}
	for _, number := range []string{"81", "82"} {
		info, err := testPublicationClient(f.c).Query(context.Background(), "https://github.com/owner/repo/pull/"+number)
		require(t, err)
		if info.State != "open" {
			t.Fatalf("PR %s was closed", number)
		}
	}
	if guard.destructive.Load() != 0 {
		t.Fatal("product attempted destructive resource operation")
	}
	ready, err := query.NewService(f.store, f.c.Identity()).ReadyNativeTasks(context.Background())
	require(t, err)
	if len(ready) != 0 {
		t.Fatal("discard unlocked successor")
	}
}

func TestPS3StopReceiptIsAtomicAndReplaysOriginalEvidence(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			service, run := neutralAuditService(t)
			db := testSQLite(service)
			ctx := context.Background()
			project := service.Config.identity
			execution := *service.executionRecord(run)
			identity := executionbackend.ProcessIdentity{Kind: "os", PID: 999999, Incarnation: "ps3-fixture"}
			execution.Process = &identity
			execution.Status = "active"
			execution.Workspace.Worktree = execution.WorkspaceIntent.Path
			execution.Workspace.Branch = execution.WorkspaceIntent.Branch
			execution.Workspace.Base = execution.WorkspaceIntent.BaseCommit
			attempt := *service.workflowAttempt(run, run.AttemptID)
			attempt.Status = "running"
			attemptID := attempt.AttemptID
			require(t, transactFixture(db.SQLiteStore, ctx, func(tx *fixtureTransaction) error {
				if err := tx.PutExecution(execution); err != nil {
					return err
				}
				return tx.PutStepAttempt(attempt)
			}))
			commands := newTestCommandService(db, service.Config)
			task := nativeTaskRecord(service.domain.Tasks, run.TaskID)
			result, err := commands.DiscardNativeTask(ctx, service.Config, task.TaskID, newID(), task.Version, "stop fixture", "operator")
			require(t, err)
			if result.Status != "discarded" {
				t.Fatalf("discard: %+v", result)
			}
			before, err := db.TaskGraphSnapshot(ctx, project)
			require(t, err)
			current := before.ByID[task.TaskID]
			ref := executionbackend.ExecutionRef{Version: executionbackend.ExecutionRecordVersion, RunID: run.RunID, AttemptID: attemptID, Backend: executionbackend.BackendKind(execution.Backend), Protocol: executionbackend.ExecutionProtocol(execution.Protocol), Target: execution.Target, Agent: AgentFor(service.Config), WorkspacePath: execution.Workspace.Worktree, WorkspaceBranch: execution.Workspace.Branch, BaseCommit: execution.Workspace.Base, Generation: execution.SessionGeneration, External: identity}
			op := executionbackend.Operation{Version: executionbackend.ExecutionRecordVersion, PayloadDigest: digestValue("controlled original stop"), ID: newID(), RunID: run.RunID, AttemptID: attemptID, Backend: ref.Backend, Protocol: ref.Protocol, Target: ref.Target, Status: executionbackend.OperationConfirmed, Action: "stop", Ref: &ref}
			evidence := NativeExecutionStopReceipt{WorkflowRunID: run.RunID, ExecutionID: execution.ExecutionID, AttemptID: attemptID, Operation: op, Observation: executionbackend.Observation{Liveness: "exited", Source: "controlled original identity", ObservedAt: time.Now().UTC()}}
			item := "execution_stop_manual:" + execution.ExecutionID
			request := newID()
			if !fail {
				bad := evidence
				altered := *op.Ref
				altered.Target.Branch = "foreign-original-branch"
				bad.Operation.Ref, bad.Operation.Target = &altered, altered.Target
				if _, err := commands.RecordNativeStopReceipt(ctx, task.TaskID, newID(), current.Version, item, "operator", mustJSON(bad)); err == nil {
					t.Fatal("same target ID with foreign branch accepted")
				}
				bad = evidence
				bad.Operation.Version = 0
				if _, err := commands.RecordNativeStopReceipt(ctx, task.TaskID, newID(), current.Version, item, "operator", mustJSON(bad)); err == nil {
					t.Fatal("invalid original operation format accepted")
				}
			}
			if fail {
				_, err = db.db.Exec(`CREATE TRIGGER reject_ps3_stop_receipt BEFORE INSERT ON commands BEGIN SELECT RAISE(FAIL,'stop receipt failure'); END`)
				require(t, err)
			}
			settled, err := commands.RecordNativeStopReceipt(ctx, task.TaskID, request, current.Version, item, "operator", mustJSON(evidence))
			after, readErr := db.TaskGraphSnapshot(ctx, project)
			require(t, readErr)
			snapshot, readErr := db.Query(ctx, project)
			require(t, readErr)
			if fail {
				if err == nil || recordFingerprint(after.ByID[task.TaskID]) != recordFingerprint(current) || after.Authority.GraphRevision != before.Authority.GraphRevision {
					t.Fatal("failed stop receipt changed task/graph")
				}
				for _, e := range snapshot.Executions {
					if e.ExecutionID == execution.ExecutionID && e.Status != "active" {
						t.Fatal("failed receipt falsely stopped execution")
					}
				}
				if _, found, e := db.LookupCommand(ctx, project, request); e != nil || found {
					t.Fatal("failed receipt published command")
				}
				return
			}
			require(t, err)
			if settled.Status != "discarded" || len(settled.StopPending) != 0 || settled.Version != current.Version+1 {
				t.Fatal("stop did not settle exact item")
			}
			replay, err := commands.RecordNativeStopReceipt(ctx, task.TaskID, request, current.Version, item, "operator", mustJSON(evidence))
			require(t, err)
			if recordFingerprint(replay) != recordFingerprint(settled) {
				t.Fatal("replay changed original result")
			}
			var saved taskmodel.TaskRecord
			command, err := db.Command(ctx, project, request)
			require(t, err)
			require(t, decode(command.Result, &saved))
			if recordFingerprint(saved) != recordFingerprint(settled) {
				t.Fatal("receipt and task differ")
			}
		})
	}
}
