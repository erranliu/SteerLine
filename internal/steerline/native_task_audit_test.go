package steerline

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tasklegacy "github.com/erranliu/SteerLine/internal/steerline/task"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func requireNativeAuditCode(t *testing.T, err error, code string) {
	t.Helper()
	var got *taskmodel.Error
	if !errors.As(err, &got) || got.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}

// Storage fixtures deliberately bypass the future DAG command/PR verification
// pipeline. Their records test persistence and query boundaries only.
func nativeAuditRecords(t *testing.T, store *sqliteFixture, a tasklegacy.ProjectTaskAuthority) []taskmodel.TaskRecord {
	t.Helper()
	created := time.Now().UTC()
	ref := taskmodel.FileReference{Path: "docs/task.spec.md", Commit: strings.Repeat("a", 40), SHA256: strings.Repeat("b", 64)}
	tasks := []taskmodel.TaskRecord{}
	err := store.transactRecords(context.Background(), func(tx *fixtureTransaction) error {
		if err := tx.PutTaskBatch(taskmodel.TaskBatchRecord{BatchID: "audit-batch", ProjectIdentity: a.ProjectIdentity, SchemaVersion: "task-batch/v1", PayloadDigest: strings.Repeat("c", 64), CommandID: "audit-command", CreatedAt: created, TaskIDs: map[string]string{"a": "2", "b": "10", "c": "99"}}, a.AuthorityVersion, a.GraphRevision); err != nil {
			return err
		}
		for _, id := range []string{"2", "10", "99"} {
			task := taskmodel.TaskRecord{ProjectIdentity: a.ProjectIdentity, TaskID: id, BatchID: "audit-batch", Title: "Audit " + id, Type: "task", Priority: 2, ContractID: "contract-" + id, Source: "native", Status: "open", Version: 1, CreatedAt: created, UpdatedAt: created}
			contract := taskmodel.TaskContract{ContractID: task.ContractID, Repository: a.Repository, Remote: a.Remote, Branch: a.Branch, SpecRef: ref, PlanRef: taskmodel.FileReference{Path: "docs/task.plan.md", Commit: ref.Commit, SHA256: ref.SHA256}, AcceptanceRefs: []taskmodel.FileReference{ref}}
			var err error
			contract.Digest, err = taskmodel.ContractDigest(contract)
			if err != nil {
				return err
			}
			if err = tx.PutTask(task); err != nil {
				return err
			}
			if err = tx.PutTaskContract(a.ProjectIdentity, id, contract); err != nil {
				return err
			}
			tasks = append(tasks, task)
		}
		return nil
	})
	require(t, err)
	return tasks
}

func nativeAuditStore(t *testing.T) (*sqliteFixture, string, []taskmodel.TaskRecord) {
	t.Helper()
	s := openDomainTestStore(t)
	a := tasklegacy.ProjectTaskAuthority{ProjectIdentity: "audit-project", Authority: "native", AuthorityVersion: 1, Repository: "https://example.invalid/Org/Repo.git", Remote: "origin", Branch: "main", DispatchMode: "manual", DispatchPaused: true}
	require(t, s.transactRecords(context.Background(), func(tx *fixtureTransaction) error { return tx.PutProjectTaskAuthority(a) }))
	return s, a.ProjectIdentity, nativeAuditRecords(t, s, a)
}

func TestNativeAuditPageBatchesRelationsAcrossPageBoundary(t *testing.T) {
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	seedNativeGraphForView(t, c, commit, store, 402, []taskmodel.TaskDependencyRecord{{ProjectIdentity: c.Identity(), PredecessorID: "1", SuccessorID: "402"}})
	q := NewQueryService(store, c.Identity())
	first, err := q.ListNativeTasks(ctx, "", 1)
	require(t, err)
	second, err := q.ListNativeTasks(ctx, first.NextCursor, 401)
	require(t, err)
	if len(second.Tasks) != 401 || second.Tasks[0].TaskID != "2" || second.Tasks[400].TaskID != "402" || second.HasMore {
		t.Fatalf("paged task order or batch boundary changed: first=%+v second-count=%d", first, len(second.Tasks))
	}
	last := second.Tasks[400]
	if last.DependencyReady || len(last.UnmetPredecessors) != 1 || last.UnmetPredecessors[0].TaskID != "1" || last.UnmetPredecessors[0].Blocker != "predecessor_not_delivered" {
		t.Fatalf("off-page predecessor was not loaded: %+v", last)
	}
	graph, err := q.TaskGraph(ctx, "", 500)
	require(t, err)
	if graph.Nodes[401].DependencyReady != last.DependencyReady || len(graph.Nodes[401].Blockers) != len(last.UnmetPredecessors) || graph.Nodes[401].Blockers[0] != last.UnmetPredecessors[0] {
		t.Fatalf("DAG and task page disagree on readiness: %+v / %+v", graph.Nodes[401], last)
	}
}

func TestNativeAuditReadersCoexistWithWriter(t *testing.T) {
	ctx := context.Background()
	c, _ := nativeTaskFixture(t)
	reader, err := OpenNativeTaskProject(ctx, &c)
	require(t, err)
	require(t, reader.Close())
	writer, err := openSQLiteFixture(filepath.Join(c.RunDir, "steerline.db"))
	require(t, err)
	defer writer.Close()
	a, err := writer.NativeAuthority(ctx, c.Identity())
	require(t, err)
	nativeAuditRecords(t, writer, a)
	lock, err := LockRepository(c.Identity())
	require(t, err)
	defer lock.Close()
	reader, err = OpenNativeTaskProject(ctx, &c)
	require(t, err)
	defer reader.Close()
	page, err := (NewQueryService(reader, c.Identity())).ListNativeTasks(ctx, "", 100)
	require(t, err)
	if len(page.Tasks) != 3 {
		t.Fatal("read failed while writer owns repository")
	}
	// Every read rechecks the binding rather than relying on an earlier handle.
	_, err = writer.db.Exec(`UPDATE project_task_authorities SET branch='other'`)
	require(t, err)
	_, err = (NewQueryService(reader, c.Identity())).ListNativeTasks(ctx, "", 100)
	requireNativeAuditCode(t, err, "authority_mismatch")
}

func TestNativeAuditSchemaDefinitionMustMatchVersionStamp(t *testing.T) {
	ctx := context.Background()
	s, _, _ := nativeAuditStore(t)
	_, err := s.db.Exec(`DROP TRIGGER task_dependencies_no_cycle`)
	require(t, err)
	requireNativeAuditCode(t, func() error {
		_, err := openSQLiteReaderFixture(func() string {
			var seq int
			var name, path string
			require(t, s.db.QueryRow("PRAGMA database_list").Scan(&seq, &name, &path))
			return path
		}())
		return err
	}(), "unsupported_schema")
	c, _ := nativeTaskFixture(t)
	reader, err := OpenNativeTaskProject(ctx, &c)
	require(t, err)
	require(t, reader.Close())
	writer, err := openSQLiteFixture(filepath.Join(c.RunDir, "steerline.db"))
	require(t, err)
	_, err = writer.db.Exec(`DROP TABLE task_deliveries`)
	require(t, err)
	require(t, writer.Close())
	_, err = OpenNativeTaskProject(ctx, &c)
	requireNativeAuditCode(t, err, "unsupported_schema")
}

func TestNativeAuditContractRolesAndFixedReferenceVersions(t *testing.T) {
	if _, err := taskmodel.CanonicalJSON([]byte{'"', 0xff, '"'}); err == nil {
		t.Fatal("invalid UTF-8 was silently replaced before hashing")
	}
	s, project, _ := nativeAuditStore(t)
	page, err := (NewQueryService(s, project)).ShowNativeTask(context.Background(), "2")
	require(t, err)
	base := page.Tasks[0].Contract
	// Each reference has its own fixed revision; naming and commit equality
	// are not pairing proofs required by TASK-CORE.
	v := base
	v.SpecRef.Path = "design.md"
	v.PlanRef.Path = "implementation.md"
	v.PlanRef.Commit = strings.Repeat("d", 40)
	v.AcceptanceRefs = []taskmodel.FileReference{v.SpecRef}
	v.CommonRefs = []taskmodel.FileReference{{Path: "docs/spec/execution-architecture.spec.md", Commit: strings.Repeat("e", 40), SHA256: strings.Repeat("f", 64)}}
	v.Digest, err = taskmodel.ContractDigest(v)
	require(t, err)
	require(t, taskmodel.ValidateContract(v))
	for _, name := range []string{"other.spec.md", "other.plan.md"} {
		v := base
		extra := base.SpecRef
		extra.Path = name
		v.AcceptanceRefs = []taskmodel.FileReference{extra}
		v.Digest, err = taskmodel.ContractDigest(v)
		require(t, err)
		requireNativeAuditCode(t, taskmodel.ValidateContract(v), "contract_mismatch")
	}
	for _, bad := range []string{"https://example.invalid/org/repo.git", "https://example.invalid/Org/Repo"} {
		if sameRepositoryTarget(base.Repository, bad) {
			t.Fatal("remote URL identity was weakened")
		}
	}
}
