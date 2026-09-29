package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkerFlowConfigRejectsOldProjectGates(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	base := map[string]any{
		"task_authority": "native", "repository": c.Repository, "branch": "main", "run_dir": c.RunDir,
		"workflow_protocol": WorkerFlowVersion, "host": "local", "agent": "codex",
	}
	write := func(value map[string]any) (Config, error) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "config.json")
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return LoadConfig(path)
	}
	loaded, err := write(base)
	if err != nil || loaded.WorkflowProtocol != WorkerFlowVersion || loaded.ProjectSteps != nil {
		t.Fatalf("new format was not accepted: config=%+v error=%v", loaded, err)
	}
	for name, field := range map[string]any{
		"workflow_definition":    map[string]any{"id": MinimalWorkflowVersion},
		"project_steps":          map[string]any{},
		"workspace_dirty_ignore": map[string]any{"paths": []string{"*.meta"}},
	} {
		t.Run(name, func(t *testing.T) {
			value := map[string]any{}
			for k, v := range base {
				value[k] = v
			}
			value[name] = field
			if _, err := write(value); err == nil {
				t.Fatal("obsolete project work gate was accepted")
			}
		})
	}
}

func TestDeferredOrcaWorkerFlowCannotClaim(t *testing.T) {
	for _, host := range []string{"orca", "orca_supervised"} {
		t.Run(host, func(t *testing.T) {
			c := Config{WorkflowProtocol: WorkerFlowVersion, Host: host, Agent: "codex"}
			err := requireWorkerFlowCapabilities(context.Background(), c)
			var backend *BackendError
			if !errors.As(err, &backend) || backend.Code != BackendUnsupported {
				t.Fatalf("deferred Orca target reached an external claim path: %v", err)
			}
		})
	}
}

func TestWorkerFlowRejectsOldRuntimeBeforeWorkspacePreparation(t *testing.T) {
	c, _, store := openDAGFixture(t)
	old := Runtime{SchemaVersion: CurrentRuntimeSchema, Repository: Repository{Path: c.Repository, Remote: c.Remote, Branch: c.Branch, Identity: c.Identity()}}
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
	if _, err := NewService(c); err == nil || !strings.Contains(err.Error(), "unsupported_schema") {
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
	service, err := NewService(c)
	if err != nil {
		t.Fatal(err)
	}
	if service.State.SchemaVersion != WorkerFlowRuntimeSchema {
		t.Fatalf("new runtime used schema %d", service.State.SchemaVersion)
	}
	if err := service.save(); err != nil {
		t.Fatal(err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewService(c)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.State.SchemaVersion != WorkerFlowRuntimeSchema {
		t.Fatalf("reopened runtime used schema %d", reopened.State.SchemaVersion)
	}
}
