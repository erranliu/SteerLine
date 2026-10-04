package sqlite

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/query"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestNativeAuditPaginationBoundsAndDatabaseIdentity(t *testing.T) {
	ctx := context.Background()
	s, project, _ := nativeAuditStore(t)
	q := NewQueryService(s, project)
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
	var cursor query.TaskCursor
	require(t, json.Unmarshal(raw, &cursor))
	cursor.Offset = int(^uint(0) >> 1)
	raw, err = json.Marshal(cursor)
	require(t, err)
	_, err = q.ListNativeTasks(ctx, base64.RawURLEncoding.EncodeToString(raw), 500)
	requireNativeAuditCode(t, err, "cursor_invalidated")
	other, _, _ := nativeAuditStore(t)
	_, err = (NewQueryService(other, project)).ListNativeTasks(ctx, first.NextCursor, 1)
	requireNativeAuditCode(t, err, "cursor_invalidated")
	// Corruption outside the requested page must not cause a full-project decode.
	_, err = s.db.Exec(`UPDATE tasks SET payload='invalid JSON' WHERE task_id='99'`)
	require(t, err)
	_, err = q.ListNativeTasks(ctx, "", 1)
	require(t, err)
	// The extra row used only to detect a next page must remain undecoded too.
	_, err = s.db.Exec(`UPDATE tasks SET payload='invalid JSON' WHERE task_id='10'`)
	require(t, err)
	_, err = q.ListNativeTasks(ctx, "", 1)
	require(t, err)
}

func TestNativeAuditPageDecodesOnlyDirectPredecessors(t *testing.T) {
	ctx := context.Background()
	store, project, _ := nativeAuditStore(t)
	require(t, store.transactSQLite(ctx, func(tx *sqliteTx) error {
		if err := tx.PutTaskDependency(taskmodel.TaskDependencyRecord{ProjectIdentity: project, PredecessorID: "2", SuccessorID: "10"}); err != nil {
			return err
		}
		return tx.PutTaskDependency(taskmodel.TaskDependencyRecord{ProjectIdentity: project, PredecessorID: "10", SuccessorID: "99"})
	}))
	_, err := store.db.ExecContext(ctx, `UPDATE tasks SET payload='invalid JSON' WHERE project_identity=? AND task_id='2'`, project)
	require(t, err)
	page, err := (NewQueryService(store, project)).ShowNativeTask(ctx, "99")
	require(t, err)
	if len(page.Tasks) != 1 || len(page.Tasks[0].UnmetPredecessors) != 1 || page.Tasks[0].UnmetPredecessors[0].TaskID != "10" {
		t.Fatalf("task detail lost its direct predecessor: %+v", page.Tasks)
	}
}

func TestNativeAuditDependencyCycleRollsBackAndEdgesAreImmutable(t *testing.T) {
	ctx := context.Background()
	s, project, _ := nativeAuditStore(t)
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		for _, edge := range [][2]string{{"2", "10"}, {"10", "99"}, {"99", "2"}} {
			if err := tx.PutTaskDependency(taskmodel.TaskDependencyRecord{ProjectIdentity: project, PredecessorID: edge[0], SuccessorID: edge[1]}); err != nil {
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
	require(t, s.transactSQLite(ctx, func(tx *sqliteTx) error {
		return tx.PutTaskDependency(taskmodel.TaskDependencyRecord{ProjectIdentity: project, PredecessorID: "2", SuccessorID: "10"})
	}))
	for _, query := range []string{`DELETE FROM task_dependencies`, `UPDATE task_dependencies SET successor_id='99'`} {
		if _, err = s.db.Exec(query); err == nil {
			t.Fatal("immutable dependency was changed")
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
	require(t, s.transactSQLite(context.Background(), func(tx *sqliteTx) error { return tx.UpdateNativeTaskCAS(task, 1) }))
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
			_, err = (NewQueryService(s, project)).ShowNativeTask(context.Background(), "2")
			requireNativeAuditCode(t, err, "contract_mismatch")
		})
	}
}
