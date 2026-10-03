package steerlineapp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	"github.com/erranliu/SteerLine/internal/steerline/console"
)

func TestFileQuerySourceReadsCombinedProjectView(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Setenv("STEERLINE_HOME", filepath.Join(root, "home"))
	if err := os.MkdirAll(os.Getenv("STEERLINE_HOME"), 0700); err != nil {
		t.Fatal(err)
	}
	localGlobalExecutionFixture(t)
	repo := filepath.Join(root, "repo")
	for _, args := range [][]string{{"init", "--quiet", "--initial-branch=main", repo}, {"-C", repo, "remote", "add", "origin", "https://github.com/Org/Repo.git"}} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git fixture: %v: %s", err, output)
		}
	}
	configPath := filepath.Join(root, "project.json")
	cfgInput := minimalConfig(repo)
	cfgInput.PR.Repository = "Org/Repo"
	if err := os.WriteFile(configPath, projectConfigJSON(t, cfgInput), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadQueryConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := steerline.OpenNativeTaskProject(ctx, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	writer, err := steerline.OpenSQLiteStore(filepath.Join(cfg.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	at := time.Now().UTC()
	err = writer.Transact(ctx, func(tx steerline.DomainTx) error {
		if err := tx.PutTaskBatch(steerline.TaskBatchRecord{ProjectIdentity: cfg.Identity(), BatchID: "source-batch", SchemaVersion: "task-batch/v1", PayloadDigest: strings.Repeat("a", 64), CommandID: "source-command", TaskIDs: map[string]string{"a": "2", "b": "10"}, CreatedAt: at}, 1, 0); err != nil {
			return err
		}
		for _, id := range []string{"2", "10"} {
			reference := steerline.FileReference{Path: "docs/task.spec.md", Commit: strings.Repeat("a", 40), SHA256: strings.Repeat("b", 64)}
			contract := steerline.TaskContract{ContractID: "contract-" + id, Repository: "https://github.com/Org/Repo.git", Remote: "origin", Branch: "main", SpecRef: reference, PlanRef: steerline.FileReference{Path: "docs/task.plan.md", Commit: reference.Commit, SHA256: reference.SHA256}, AcceptanceRefs: []steerline.FileReference{reference}}
			var err error
			contract.Digest, err = steerline.TaskContractDigest(contract)
			if err != nil {
				return err
			}
			task := steerline.TaskRecord{ProjectIdentity: cfg.Identity(), TaskID: id, BatchID: "source-batch", ContractID: contract.ContractID, Title: "Task " + id, Type: "task", Priority: 2, Source: "native", Status: "open", Version: 1, CreatedAt: at, UpdatedAt: at}
			if err := tx.PutTask(task); err != nil {
				return err
			}
			if err := tx.PutTaskContract(cfg.Identity(), id, contract); err != nil {
				return err
			}
		}
		return tx.PutTaskDependency(steerline.TaskDependencyRecord{ProjectIdentity: cfg.Identity(), PredecessorID: "2", SuccessorID: "10"})
	})
	if err != nil {
		t.Fatal(err)
	}
	ref := console.ProjectRef{Key: "project", ConfigPath: configPath, Config: cfg, Identity: cfg.Identity(), RunDir: cfg.RunDir}
	factory := func(ctx context.Context, ref api.ProjectRef) (api.Resources, error) {
		copy := cfg
		copy.RunDir = ref.RunDir
		return clientResources(ctx, copy, ref)
	}
	id, revision, err := writer.DatabaseVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := (console.FileQuerySource{Factory: factory}).ReadProject(ctx, ref, "10", console.ProjectSnapshot{})
	if snapshot.TaskError != "" || snapshot.NativeTaskError != "" || snapshot.DomainError != "" || snapshot.FullReadAt.IsZero() ||
		snapshot.NativeTasks == nil || len(snapshot.NativeTasks.Tasks) != 2 || snapshot.NativeTasks.DatabaseRevision != revision || snapshot.NativeTasks.DatabaseID != id ||
		snapshot.SelectedNative == nil || snapshot.SelectedNative.TaskID != "10" || snapshot.SelectedNative.DependencyReady ||
		snapshot.Tasks == nil || len(snapshot.Tasks.Tasks) != 1 || snapshot.Tasks.Tasks[0].TaskID != "2" || snapshot.Domain == nil || len(snapshot.Domain.Tasks) != 2 ||
		snapshot.Observation.State != steerline.SnapshotNone || snapshot.Connection.State != steerline.ConnectionNoRecord || !snapshot.Connection.CanStart ||
		!snapshot.FullReadAt.Equal(snapshot.NativeTasks.SampledAt) || !snapshot.Observation.SampledAt.Equal(snapshot.NativeTasks.SampledAt) {
		t.Fatalf("console did not consume the combined project view: %+v", snapshot)
	}
	nextID, nextRevision, err := writer.DatabaseVersion(ctx)
	if err != nil || nextID != id || nextRevision != revision {
		t.Fatal("read-only console refresh modified the database")
	}
	missing := (console.FileQuerySource{Factory: factory}).ReadProject(ctx, ref, "99", console.ProjectSnapshot{})
	if missing.NativeTaskError == "" || !missing.FullReadAt.IsZero() || missing.NativeTasks == nil || missing.Tasks == nil || missing.Domain == nil {
		t.Fatal("missing selection discarded read-only data or made the refresh reusable")
	}
	if err := writer.Transact(ctx, func(tx steerline.DomainTx) error {
		return tx.PutProjection(cfg.Identity(), 1, []byte("invalid JSON"))
	}); err != nil {
		t.Fatal(err)
	}
	corrupt := (console.FileQuerySource{Factory: factory}).ReadProject(ctx, ref, "2", console.ProjectSnapshot{})
	if corrupt.Connection.State != steerline.ConnectionCorrupt || corrupt.Connection.CanStart || corrupt.Connection.CanControl ||
		corrupt.Observation.Runtime != nil || corrupt.TaskError == "" || !corrupt.FullReadAt.IsZero() {
		t.Fatal("reader preflight admitted a corrupt service checkpoint")
	}
	absent := ref
	absent.Config.RunDir = filepath.Join(root, "missing")
	absent.RunDir = absent.Config.RunDir
	failed := (console.FileQuerySource{Factory: factory}).ReadProject(ctx, absent, "", console.ProjectSnapshot{})
	if failed.TaskError == "" || failed.DomainError == "" || !failed.FullReadAt.IsZero() {
		t.Fatal("missing database was shown as a successful refresh")
	}
	if _, err := os.Stat(absent.RunDir); !os.IsNotExist(err) {
		t.Fatal("console created a missing project state directory")
	}
}
