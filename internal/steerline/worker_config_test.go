package steerline

import (
	"context"
	"encoding/json"
	"errors"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnregisteredWorkerFlowConfigurationCannotClaim(t *testing.T) {
	for _, host := range []string{"unregistered-a", "unregistered-b"} {
		t.Run(host, func(t *testing.T) {
			c := Config{ExecutionDirectory: neutralExecutionDirectory(t), WorkflowProtocol: WorkerFlowVersion, Host: host, Agent: "fixture"}
			err := requireWorkerFlowCapabilities(context.Background(), c)
			var backend *executionapi.BackendError
			if !errors.As(err, &backend) || backend.Code != executionapi.BackendUnsupported {
				t.Fatalf("unregistered configuration reached an external claim path: %v", err)
			}
		})
	}
}

func TestWorkerFlowRejectsOldRuntimeBeforeWorkspacePreparation(t *testing.T) {
	c, _, store := openDAGFixture(t)
	old := Runtime{SchemaVersion: 5, Repository: Repository{Path: c.Repository, Remote: c.Remote, Branch: c.Branch, Identity: c.Identity()}}
	payload, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Transact(context.Background(), func(tx DomainTx) error {
		return tx.PutProjection(c.Identity(), 1, payload)
	}); err != nil {
		t.Fatal(err)
	}
	c.WorkflowProtocol = WorkerFlowVersion
	workspaceRoot := filepath.Join(c.RunDir, "worktrees")
	if _, err := newTestService(c); err == nil || !strings.Contains(err.Error(), "unsupported_schema") {
		t.Fatalf("old state was not rejected: %v", err)
	}
	if _, err := os.Stat(workspaceRoot); !os.IsNotExist(err) {
		t.Fatalf("old state preflight prepared a workspace: %v", err)
	}
	saved, _, err := store.LoadProjection(context.Background(), c.Identity())
	if err != nil || string(saved) != string(payload) {
		t.Fatal("old state was changed by the rejected start")
	}
}

func TestFreshWorkerFlowRuntimeUsesDistinctFormat(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	c.WorkflowProtocol = WorkerFlowVersion
	project, err := OpenNativeTaskProject(context.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	if err := project.Close(); err != nil {
		t.Fatal(err)
	}
	service, err := newTestService(c)
	if err != nil {
		t.Fatal(err)
	}
	if service.State.SchemaVersion != WorkerFlowRuntimeSchema {
		t.Fatalf("new runtime used schema %d", service.State.SchemaVersion)
	}
	if err := service.publishCheckpoint(); err != nil {
		t.Fatal(err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := newTestService(c)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.State.SchemaVersion != WorkerFlowRuntimeSchema {
		t.Fatalf("reopened runtime used schema %d", reopened.State.SchemaVersion)
	}
}

func TestWorkerFlowRejectsOldLocalExecutionProtocol(t *testing.T) {
	c, commit, store := openDAGFixture(t)
	ctx := context.Background()
	c.WorkflowProtocol, c.Host, c.Agent, c.Claimant = WorkerFlowVersion, "local", "fixture", "operator"
	if out, err := exec.Command("git", "-C", c.Repository, "push", "--quiet", "origin", "HEAD:refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("push fixture base: %v: %s", err, out)
	}
	commands := CommandService{Store: store, Project: c.Identity()}
	if _, err := commands.CreateTaskBatch(ctx, c, taskBatchFixture(t, c, commit, batchTasks(8), nil), strings.Repeat("c", 32), 1, "operator"); err != nil {
		t.Fatal(err)
	}
	authority, err := store.nativeAuthority(ctx, c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	authority.AuthorityVersion++
	authority.DispatchPaused = false
	if err := store.Transact(ctx, func(tx DomainTx) error {
		return tx.(*sqliteTx).UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
	}); err != nil {
		t.Fatal(err)
	}
	receipt, err := commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: "8", RequestID: strings.Repeat("d", 32), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: authority.AuthorityVersion, RequestedBy: "operator", Explicit: true})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := OpenSQLiteReader(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	snapshot, err := reader.Query(ctx, c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	var old ExecutionRecord
	for _, execution := range snapshot.Executions {
		if execution.ExecutionID == receipt.ExecutionID {
			old = execution
		}
	}
	if old.ExecutionID == "" {
		t.Fatal("claimed execution is missing")
	}
	old.Protocol = string(executionapi.ProtocolLocalProcess)
	if err := store.Transact(ctx, func(tx DomainTx) error { return tx.PutExecution(old) }); err != nil {
		t.Fatal(err)
	}
	workspaceRoot := filepath.Join(c.RunDir, "worktrees")
	if _, err := newTestService(c); err == nil || !strings.Contains(err.Error(), "unsupported_schema") {
		t.Fatalf("old local execution was not rejected: %v", err)
	}
	if _, err := os.Stat(workspaceRoot); !os.IsNotExist(err) {
		t.Fatalf("old local execution preflight prepared a workspace: %v", err)
	}
	snapshot, err = reader.Query(ctx, c.Identity())
	if err != nil || len(snapshot.Executions) != 1 || snapshot.Executions[0].Protocol != old.Protocol {
		t.Fatalf("rejected old execution was changed: %+v %v", snapshot.Executions, err)
	}
}
