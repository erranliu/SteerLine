package sqlite

import (
	"crypto/sha256"
	"encoding/hex"
)
import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/query"
	taskrules "github.com/erranliu/SteerLine/internal/steerline/task"
)

func require(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func NewQueryService(s query.Store, p string) query.Service { return query.NewService(s, p) }
func requireNativeAuditCode(t *testing.T, err error, code string) {
	t.Helper()
	var got *taskrules.Error
	if !errors.As(err, &got) || got.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}

// Storage fixtures deliberately bypass the future DAG command/PR verification
// pipeline. Their records test persistence and query boundaries only.
func nativeAuditRecords(t *testing.T, store *SQLiteStore, a ProjectTaskAuthority) []TaskRecord {
	t.Helper()
	created := time.Now().UTC()
	ref := taskrules.FileReference{Path: "docs/task.spec.md", Commit: strings.Repeat("a", 40), SHA256: strings.Repeat("b", 64)}
	tasks := []TaskRecord{}
	err := store.transactSQLite(context.Background(), func(tx *sqliteTx) error {
		if err := tx.PutTaskBatch(TaskBatchRecord{BatchID: "audit-batch", ProjectIdentity: a.ProjectIdentity, SchemaVersion: "task-batch/v1", PayloadDigest: strings.Repeat("c", 64), CommandID: "audit-command", CreatedAt: created, TaskIDs: map[string]string{"a": "2", "b": "10", "c": "99"}}, a.AuthorityVersion, a.GraphRevision); err != nil {
			return err
		}
		for _, id := range []string{"2", "10", "99"} {
			task := TaskRecord{ProjectIdentity: a.ProjectIdentity, TaskID: id, BatchID: "audit-batch", Title: "Audit " + id, Type: "task", Priority: 2, ContractID: "contract-" + id, Source: "native", Status: "open", Version: 1, CreatedAt: created, UpdatedAt: created}
			contract := TaskContract{ContractID: task.ContractID, Repository: a.Repository, Remote: a.Remote, Branch: a.Branch, SpecRef: ref, PlanRef: taskrules.FileReference{Path: "docs/task.plan.md", Commit: ref.Commit, SHA256: ref.SHA256}, AcceptanceRefs: []taskrules.FileReference{ref}}
			var err error
			contract.Digest, err = taskrules.ContractDigest(contract)
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
	require(t, s.transactSQLite(context.Background(), func(tx *sqliteTx) error { return tx.PutProjectTaskAuthority(a) }))
	return s, a.ProjectIdentity, nativeAuditRecords(t, s, a)
}

func conversationQueries(s *SQLiteStore, p string) query.ConversationService {
	return query.ConversationService{Project: p, Events: s, Bodies: s, Raw: s, Capabilities: s}
}
func conversationItemKey(e ConversationEvent, id string) string {
	if id == "" {
		return ""
	}
	e.ItemID = id
	h := sha256.Sum256([]byte(e.ItemIdentity()))
	return hex.EncodeToString(h[:])
}
