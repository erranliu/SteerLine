package steerline

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func requireNativeAuditCode(t *testing.T, err error, code string) {
	t.Helper()
	var got *NativeTaskError
	if !errors.As(err, &got) || got.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}

// Storage fixtures deliberately bypass the future DAG command/PR verification
// pipeline. Their records test persistence and query boundaries only.
func nativeAuditRecords(t *testing.T, store *SQLiteStore, a ProjectTaskAuthority) []TaskRecord {
	t.Helper()
	created := time.Now().UTC()
	ref := FileReference{Path: "docs/task.spec.md", Commit: strings.Repeat("a", 40), SHA256: strings.Repeat("b", 64)}
	tasks := []TaskRecord{}
	err := store.Transact(context.Background(), func(tx DomainTx) error {
		if err := tx.PutTaskBatch(TaskBatchRecord{BatchID: "audit-batch", ProjectIdentity: a.ProjectIdentity, SchemaVersion: "task-batch/v1", PayloadDigest: strings.Repeat("c", 64), CommandID: "audit-command", CreatedAt: created, TaskIDs: map[string]string{"a": "2", "b": "10", "c": "99"}}, a.AuthorityVersion, a.GraphRevision); err != nil {
			return err
		}
		for _, id := range []string{"2", "10", "99"} {
			task := TaskRecord{ProjectIdentity: a.ProjectIdentity, TaskID: id, BatchID: "audit-batch", Title: "Audit " + id, Type: "task", Priority: 2, ContractID: "contract-" + id, Source: "native", Status: "open", Version: 1, CreatedAt: created, UpdatedAt: created}
			contract := TaskContract{ContractID: task.ContractID, Repository: a.Repository, Remote: a.Remote, Branch: a.Branch, SpecRef: ref, PlanRef: FileReference{Path: "docs/task.plan.md", Commit: ref.Commit, SHA256: ref.SHA256}, AcceptanceRefs: []FileReference{ref}}
			var err error
			contract.Digest, err = TaskContractDigest(contract)
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

func nativeAuditStore(t *testing.T) (*SQLiteStore, string, []TaskRecord) {
	t.Helper()
	s := openDomainTestStore(t)
	a := ProjectTaskAuthority{ProjectIdentity: "audit-project", Authority: "native", AuthorityVersion: 1, Repository: "https://example.invalid/Org/Repo.git", Remote: "origin", Branch: "main", DispatchMode: "manual", DispatchPaused: true}
	require(t, s.Transact(context.Background(), func(tx DomainTx) error { return tx.PutProjectTaskAuthority(a) }))
	return s, a.ProjectIdentity, nativeAuditRecords(t, s, a)
}

func TestNativeAuditPaginationBoundsAndDatabaseIdentity(t *testing.T) {
	ctx := context.Background()
	s, project, _ := nativeAuditStore(t)
	q := QueryService{Store: s, Project: project}
	first, err := q.ListNativeTasks(ctx, "", 1)
	require(t, err)
	if first.Tasks[0].TaskID != "2" || first.NextCursor == "" || first.Capabilities.StartupBlockers != "provided" {
		t.Fatalf("invalid page: %+v", first)
	}
	second, err := q.ListNativeTasks(ctx, first.NextCursor, 1)
	require(t, err)
	if second.Tasks[0].TaskID != "10" {
		t.Fatal("numeric pagination order changed")
	}
	for _, offset := range []int{-1, int(^uint(0) >> 1)} {
		_, err = s.NativeTaskPage(ctx, project, "", "", 0, offset, 500)
		requireNativeAuditCode(t, err, "cursor_invalidated")
	}
	raw, err := base64.RawURLEncoding.DecodeString(first.NextCursor)
	require(t, err)
	var cursor nativeTaskCursor
	require(t, json.Unmarshal(raw, &cursor))
	cursor.Offset = int(^uint(0) >> 1)
	raw, err = json.Marshal(cursor)
	require(t, err)
	_, err = q.ListNativeTasks(ctx, base64.RawURLEncoding.EncodeToString(raw), 500)
	requireNativeAuditCode(t, err, "cursor_invalidated")
	other, _, _ := nativeAuditStore(t)
	_, err = (QueryService{Store: other, Project: project}).ListNativeTasks(ctx, first.NextCursor, 1)
	requireNativeAuditCode(t, err, "cursor_invalidated")
	// Corruption outside the requested page must not cause a full-project decode.
	_, err = s.db.Exec(`UPDATE tasks SET payload='invalid JSON' WHERE task_id='99'`)
	require(t, err)
	_, err = q.ListNativeTasks(ctx, "", 1)
	require(t, err)
}

func TestNativeAuditReadersCoexistWithWriterAndViewerUsesNative(t *testing.T) {
	ctx := context.Background()
	c, _ := nativeTaskFixture(t)
	reader, err := OpenNativeTaskProject(ctx, &c)
	require(t, err)
	require(t, reader.Close())
	writer, err := OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
	require(t, err)
	defer writer.Close()
	a, err := writer.nativeAuthority(ctx, c.Identity())
	require(t, err)
	nativeAuditRecords(t, writer, a)
	lock, err := LockRepository(c.Identity())
	require(t, err)
	defer lock.Close()
	reader, err = OpenNativeTaskProject(ctx, &c)
	require(t, err)
	defer reader.Close()
	page, err := (QueryService{Store: reader, Project: c.Identity()}).ListNativeTasks(ctx, "", 100)
	require(t, err)
	if len(page.Tasks) != 3 {
		t.Fatal("read failed while writer owns repository")
	}
	c.BD = Command{Executable: "must-never-run-bd"}
	detail, err := LoadTaskDetail(ctx, c, "2")
	require(t, err)
	if detail.ID != "2" || !strings.Contains(detail.Description, "Sampled:") {
		t.Fatalf("native viewer detail is incomplete: %+v", detail)
	}
	// Every read rechecks the binding rather than relying on an earlier handle.
	_, err = writer.db.Exec(`UPDATE project_task_authorities SET branch='other'`)
	require(t, err)
	_, err = (QueryService{Store: reader, Project: c.Identity()}).ListNativeTasks(ctx, "", 100)
	requireNativeAuditCode(t, err, "authority_mismatch")
}

func TestNativeAuditSchemaDefinitionMustMatchVersionStamp(t *testing.T) {
	ctx := context.Background()
	s, _, _ := nativeAuditStore(t)
	_, err := s.db.Exec(`DROP TRIGGER task_dependencies_no_cycle`)
	require(t, err)
	requireNativeAuditCode(t, validateDatabaseSchema(ctx, s.db), "unsupported_schema")
	c, _ := nativeTaskFixture(t)
	reader, err := OpenNativeTaskProject(ctx, &c)
	require(t, err)
	require(t, reader.Close())
	writer, err := OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
	require(t, err)
	_, err = writer.db.Exec(`DROP TABLE task_deliveries`)
	require(t, err)
	require(t, writer.Close())
	_, err = OpenNativeTaskProject(ctx, &c)
	requireNativeAuditCode(t, err, "unsupported_schema")
}

func TestNativeAuditDependencyCycleRollsBackAndEdgesAreImmutable(t *testing.T) {
	ctx := context.Background()
	s, project, _ := nativeAuditStore(t)
	err := s.Transact(ctx, func(tx DomainTx) error {
		for _, edge := range [][2]string{{"2", "10"}, {"10", "99"}, {"99", "2"}} {
			if err := tx.PutTaskDependency(TaskDependencyRecord{ProjectIdentity: project, PredecessorID: edge[0], SuccessorID: edge[1]}); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "dependency_cycle") {
		t.Fatalf("cycle was accepted: %v", err)
	}
	var count int
	require(t, s.db.QueryRow(`SELECT COUNT(*) FROM task_dependencies`).Scan(&count))
	if count != 0 {
		t.Fatal("failed dependency transaction left partial edges")
	}
	require(t, s.Transact(ctx, func(tx DomainTx) error {
		return tx.PutTaskDependency(TaskDependencyRecord{ProjectIdentity: project, PredecessorID: "2", SuccessorID: "10"})
	}))
	for _, query := range []string{`DELETE FROM task_dependencies`, `UPDATE task_dependencies SET successor_id='99'`} {
		if _, err = s.db.Exec(query); err == nil {
			t.Fatal("immutable dependency was changed")
		}
	}
}

func TestNativeAuditContractRolesAndFixedReferenceVersions(t *testing.T) {
	if _, err := CanonicalJSON([]byte{'"', 0xff, '"'}); err == nil {
		t.Fatal("invalid UTF-8 was silently replaced before hashing")
	}
	s, project, _ := nativeAuditStore(t)
	page, err := (QueryService{Store: s, Project: project}).ShowNativeTask(context.Background(), "2")
	require(t, err)
	base := page.Tasks[0].Contract
	// Each reference has its own fixed revision; naming and commit equality
	// are not pairing proofs required by TASK-CORE.
	v := base
	v.SpecRef.Path = "design.md"
	v.PlanRef.Path = "implementation.md"
	v.PlanRef.Commit = strings.Repeat("d", 40)
	v.AcceptanceRefs = []FileReference{v.SpecRef}
	v.CommonRefs = []FileReference{{Path: "docs/spec/execution-architecture.spec.md", Commit: strings.Repeat("e", 40), SHA256: strings.Repeat("f", 64)}}
	v.Digest, err = TaskContractDigest(v)
	require(t, err)
	require(t, ValidateTaskContract(v))
	for _, name := range []string{"other.spec.md", "other.plan.md"} {
		v := base
		extra := base.SpecRef
		extra.Path = name
		v.AcceptanceRefs = []FileReference{extra}
		v.Digest, err = TaskContractDigest(v)
		require(t, err)
		requireNativeAuditCode(t, ValidateTaskContract(v), "contract_mismatch")
	}
	for _, bad := range []string{"https://example.invalid/org/repo.git", "https://example.invalid/Org/Repo"} {
		if sameRepositoryTarget(base.Repository, bad) {
			t.Fatal("remote URL identity was weakened")
		}
	}
}

func TestNativeAuditTaskCASComparesInstantsNotTimeRepresentation(t *testing.T) {
	s, _, tasks := nativeAuditStore(t)
	task := tasks[0]
	task.CreatedAt = task.CreatedAt.In(time.FixedZone("UTC+8", 8*60*60))
	task.Version++
	task.Status = "in_progress"
	task.Claimant = "auditor"
	require(t, s.Transact(context.Background(), func(tx DomainTx) error { return tx.UpdateNativeTaskCAS(task, 1) }))
}

func TestNativeAuditQueryRejectsPayloadIdentityDrift(t *testing.T) {
	for _, field := range []string{"project_identity", "version"} {
		t.Run(field, func(t *testing.T) {
			s, project, _ := nativeAuditStore(t)
			var value any = "another-project"
			if field == "version" {
				value = 99
			}
			_, err := s.db.Exec(`UPDATE tasks SET payload=json_set(payload,?,?) WHERE task_id='2'`, "$."+field, value)
			require(t, err)
			_, err = (QueryService{Store: s, Project: project}).ShowNativeTask(context.Background(), "2")
			requireNativeAuditCode(t, err, "contract_mismatch")
		})
	}
}
