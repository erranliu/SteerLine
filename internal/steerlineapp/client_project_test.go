package steerlineapp

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	productclient "github.com/erranliu/SteerLine/internal/product/client"
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	"github.com/erranliu/SteerLine/internal/steerline/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
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
	writer, err := sqlite.OpenSQLiteStore(filepath.Join(cfg.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	at := time.Now().UTC()
	err = writer.TransactTaskBatch(ctx, func(tx persistence.BatchTx) error {
		if err := tx.PutTaskBatch(taskmodel.TaskBatchRecord{ProjectIdentity: cfg.Identity(), BatchID: "source-batch", SchemaVersion: "task-batch/v1", PayloadDigest: strings.Repeat("a", 64), CommandID: "source-command", TaskIDs: map[string]string{"a": "2", "b": "10"}, CreatedAt: at}, 1, 0); err != nil {
			return err
		}
		for _, id := range []string{"2", "10"} {
			reference := taskmodel.FileReference{Path: "docs/task.spec.md", Commit: strings.Repeat("a", 40), SHA256: strings.Repeat("b", 64)}
			contract := taskmodel.TaskContract{ContractID: "contract-" + id, Repository: "https://github.com/Org/Repo.git", Remote: "origin", Branch: "main", SpecRef: reference, PlanRef: taskmodel.FileReference{Path: "docs/task.plan.md", Commit: reference.Commit, SHA256: reference.SHA256}, AcceptanceRefs: []taskmodel.FileReference{reference}}
			var err error
			contract.Digest, err = taskmodel.ContractDigest(contract)
			if err != nil {
				return err
			}
			task := taskmodel.TaskRecord{ProjectIdentity: cfg.Identity(), TaskID: id, BatchID: "source-batch", ContractID: contract.ContractID, Title: "Task " + id, Type: "task", Priority: 2, Source: "native", Status: "open", Version: 1, CreatedAt: at, UpdatedAt: at}
			if err := tx.PutTask(task); err != nil {
				return err
			}
			if err := tx.PutTaskContract(cfg.Identity(), id, contract); err != nil {
				return err
			}
		}
		return tx.PutTaskDependency(taskmodel.TaskDependencyRecord{ProjectIdentity: cfg.Identity(), PredecessorID: "2", SuccessorID: "10"})
	})
	if err != nil {
		t.Fatal(err)
	}
	ref := api.ProjectRef{Key: "project", ConfigPath: configPath, Identity: cfg.Identity(), RunDir: cfg.RunDir}
	factory := func(ctx context.Context, ref api.ProjectRef) (productclient.Resources, error) {
		copy := cfg
		copy.RunDir = ref.RunDir
		return clientResources(ctx, copy, ref)
	}
	id, revision, err := writer.DatabaseVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client, err := productclient.Open(ctx, ref, factory)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_, missingTaskErr := client.ShowTask(ctx, "999")
	var publicError *api.Error
	var domainError *taskmodel.Error
	if !errors.As(missingTaskErr, &publicError) || publicError.Kind != api.UnknownRecord || !errors.As(missingTaskErr, &domainError) || domainError.Code != "task_not_found" {
		t.Fatalf("real missing task lost public error or original cause: %v", missingTaskErr)
	}

	snapshot := client.ReadProject(ctx, "10", api.ProjectSnapshot{})
	if snapshot.TaskError != "" || snapshot.NativeTaskError != "" || snapshot.HistoryError != "" || snapshot.FullReadAt.IsZero() ||
		snapshot.NativeTasks == nil || len(snapshot.NativeTasks.Tasks) != 2 || snapshot.NativeTasks.DatabaseRevision != revision || snapshot.NativeTasks.DatabaseID != id ||
		snapshot.SelectedNative == nil || snapshot.SelectedNative.TaskID != "10" || snapshot.SelectedNative.DependencyReady ||
		snapshot.Tasks == nil || len(snapshot.Tasks.Tasks) != 1 || snapshot.Tasks.Tasks[0].TaskID != "2" || snapshot.History == nil || len(snapshot.History.WorkflowRuns) != 0 ||
		snapshot.Observation.State != steerline.SnapshotNone || snapshot.Connection.State != steerline.ConnectionNoRecord || !snapshot.Connection.CanStart ||
		!snapshot.FullReadAt.Equal(snapshot.NativeTasks.SampledAt) || !snapshot.Observation.SampledAt.Equal(snapshot.NativeTasks.SampledAt) {
		t.Fatalf("console did not consume the combined project view: %+v", snapshot)
	}
	nextID, nextRevision, err := writer.DatabaseVersion(ctx)
	if err != nil || nextID != id || nextRevision != revision {
		t.Fatal("read-only console refresh modified the database")
	}
	missing := client.ReadProject(ctx, "99", api.ProjectSnapshot{})
	if missing.NativeTaskError == "" || !missing.FullReadAt.IsZero() || missing.NativeTasks == nil || missing.Tasks == nil || missing.History == nil {
		t.Fatal("missing selection discarded read-only data or made the refresh reusable")
	}
	// Corruption is a test-only direct database mutation; no product writer
	// accepts an arbitrary checkpoint payload.
	corruptDB, err := sql.Open("sqlite", filepath.Join(cfg.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = corruptDB.ExecContext(ctx, `INSERT INTO projection(project_identity,revision,payload,updated_at) VALUES(?,?,?,?)`, cfg.Identity(), 1, []byte("invalid JSON"), at.Format(time.RFC3339Nano))
	closeErr := corruptDB.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	corrupt := client.ReadProject(ctx, "2", api.ProjectSnapshot{})
	if corrupt.Connection.State != steerline.ConnectionCorrupt || corrupt.Connection.CanStart || corrupt.Connection.CanControl ||
		corrupt.Observation.Page != nil || corrupt.TaskError == "" || !corrupt.FullReadAt.IsZero() {
		t.Fatalf("reader preflight admitted a corrupt service checkpoint: %+v", corrupt)
	}
	absent := ref
	absent.RunDir = filepath.Join(root, "missing")
	absentClient, err := productclient.Open(ctx, absent, factory)
	if err != nil {
		t.Fatal(err)
	}
	defer absentClient.Close()
	failed := absentClient.ReadProject(ctx, "", api.ProjectSnapshot{})
	if failed.TaskError == "" || failed.HistoryError == "" || !failed.FullReadAt.IsZero() {
		t.Fatal("missing database was shown as a successful refresh")
	}
	if _, err := os.Stat(absent.RunDir); !os.IsNotExist(err) {
		t.Fatal("console created a missing project state directory")
	}
}
