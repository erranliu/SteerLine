package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	"github.com/erranliu/SteerLine/internal/steerline/query"
	readsqlite "github.com/erranliu/SteerLine/internal/steerline/sqlite"
	tasklegacy "github.com/erranliu/SteerLine/internal/steerline/task"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestReaderPortsUseCurrentDatabaseAndPreserveReadOnlyBinding(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "steerline.db")
	writer, err := readsqlite.OpenSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	authority := tasklegacy.ProjectTaskAuthority{ProjectIdentity: "project", Authority: "native", AuthorityVersion: 1, Repository: "https://example.invalid/repo.git", Remote: "origin", Branch: "main", DispatchMode: "manual", DispatchPaused: true}
	at := time.Now().UTC()
	large := []byte(strings.Repeat("x", query.MaxEventPageBytes))
	if err = writer.InitializeProject(ctx, authority); err != nil {
		t.Fatal(err)
	}
	err = writer.TransactTaskBatch(ctx, func(tx persistence.BatchTx) error {
		if err := tx.PutTaskBatch(taskmodel.TaskBatchRecord{ProjectIdentity: "project", BatchID: "batch", SchemaVersion: "task-batch/v1", PayloadDigest: strings.Repeat("a", 64), CommandID: "command", TaskIDs: map[string]string{"a": "1", "b": "2"}, CreatedAt: at}, 1, 0); err != nil {
			return err
		}
		for _, id := range []string{"1", "2"} {
			ref := taskmodel.FileReference{Path: "docs/task.spec.md", Commit: strings.Repeat("a", 40), SHA256: strings.Repeat("b", 64)}
			contract := taskmodel.TaskContract{ContractID: "contract-" + id, Repository: authority.Repository, Remote: authority.Remote, Branch: authority.Branch, SpecRef: ref, PlanRef: taskmodel.FileReference{Path: "docs/task.plan.md", Commit: ref.Commit, SHA256: ref.SHA256}, AcceptanceRefs: []taskmodel.FileReference{ref}}
			contract.Digest, err = taskmodel.ContractDigest(contract)
			if err != nil {
				return err
			}
			record := taskmodel.TaskRecord{ProjectIdentity: "project", TaskID: id, BatchID: "batch", ContractID: contract.ContractID, Title: "Task " + id, Type: "task", Priority: 2, Source: "native", Status: "open", Version: 1, CreatedAt: at, UpdatedAt: at}
			if err := tx.PutTask(record); err != nil {
				return err
			}
			if err := tx.PutTaskContract("project", id, contract); err != nil {
				return err
			}
		}
		if err := tx.PutTaskDependency(taskmodel.TaskDependencyRecord{ProjectIdentity: "project", PredecessorID: "1", SuccessorID: "2"}); err != nil {
			return err
		}
		for i, body := range [][]byte{large, []byte("tail")} {
			if _, err := tx.AppendEvent(storageports.DomainEvent{ProjectIdentity: "project", ExecutionID: "execution", ExternalSource: "reader", ExternalID: fmt.Sprint(i), Type: "message", Payload: body}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.SaveCheckpoint(ctx, contracts.ServiceCheckpoint{SchemaVersion: contracts.WorkerFlowRuntimeSchema, Repository: contracts.Repository{Identity: "project"}, Service: contracts.ServiceState{InstanceID: "sample"}, HeartbeatAt: at}); err != nil {
		t.Fatal(err)
	}
	uriPath := filepath.ToSlash(path)
	if runtime.GOOS == "windows" {
		uriPath = "/" + uriPath
	}
	uri := url.URL{Scheme: "file", Path: uriPath}
	db, err := sql.Open("sqlite", uri.String()+"?mode=ro&_pragma=query_only(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	mutableBinding := authority
	reader := readsqlite.NewReader(db, &mutableBinding)
	mutableBinding.Branch = "changed-after-opening"
	identity, revision, err := reader.DatabaseVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	service := query.Service{Project: "project", Workflows: reader, Tasks: reader, Events: reader}
	combined, err := query.NewService(reader, "project").ReadProjectView(ctx, "2", 1)
	if err != nil || combined.Runtime == nil || combined.RuntimeError != nil || combined.Domain == nil || combined.SelectedTask == nil || combined.SelectedTask.TaskID != "2" {
		t.Fatalf("SQLite reader could not independently compose the project view: %+v %v", combined, err)
	}
	combined.Runtime.Tasks[0].Title = "view-only"
	if combined.Domain.Tasks[0].Title == "view-only" {
		t.Fatal("project Runtime aliases the separately returned domain snapshot")
	}
	ready, err := service.ReadyNativeTasks(ctx)
	if err != nil || len(ready) != 1 || ready[0].TaskID != "1" {
		t.Fatalf("independent task reader lost readiness: %+v %v", ready, err)
	}
	page, err := service.ReadEvents(ctx, query.EventFilter{ExecutionID: "execution"}, "", 200)
	if err != nil || len(page.Events) != 1 || !page.HasMore || len(page.Events[0].Payload) != query.MaxEventPageBytes {
		t.Fatalf("independent event reader lost the byte budget: %+v %v", page, err)
	}
	last, err := service.ReadEvents(ctx, query.EventFilter{ExecutionID: "execution"}, page.NextCursor, 200)
	if err != nil || len(last.Events) != 1 || last.HasMore || string(last.Events[0].Payload) != "tail" {
		t.Fatalf("event reader skipped a prefetched event: %+v %v", last, err)
	}
	records, err := reader.ReadProjectRecords(ctx, "project", "2", 1)
	if err != nil || records.View.TaskError != nil || records.View.DomainError != nil || records.View.RuntimeError != nil || records.View.SelectedTask == nil || records.View.SelectedTask.TaskID != "2" ||
		records.View.Tasks.DatabaseRevision != revision || records.View.Ready.DatabaseRevision != revision || records.Metadata == nil || records.Metadata.Service.InstanceID != "sample" || records.View.Runtime != nil {
		t.Fatalf("raw project reader lost the shared sample or performed application projection: %+v %v", records, err)
	}
	changed := authority
	changed.Branch = "other"
	_, err = readsqlite.NewReader(db, &changed).NativeTaskPage(ctx, "project", "", "", 0, 0, 1)
	var coded *taskmodel.Error
	if !errors.As(err, &coded) || coded.Code != "authority_mismatch" {
		t.Fatalf("bound reader accepted a different target: %v", err)
	}
	currentIdentity, currentRevision, err := reader.DatabaseVersion(ctx)
	if err != nil || currentIdentity != identity || currentRevision != revision {
		t.Fatal("reader queries changed the authoritative database")
	}
}
