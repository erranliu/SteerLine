package host

import (
	"context"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

func TestAddNativeTaskPredecessorsIsAtomicIdempotentAndBlocksClaim(t *testing.T) {
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	commands := newTestCommandService(store, c.Identity())
	batch := taskBatchFixture(t, c, commit, batchTasks(101, 102, 103), nil)
	_, err := commands.CreateTaskBatch(ctx, c, batch, newID(), 1, "planner")
	require(t, err)
	input := productapi.TaskPredecessorAddInput{TaskID: "102", PredecessorIDs: []string{"103", "101"}, RequestID: newID(), ExpectedTaskVersion: 1, ExpectedGraphRevision: 1, Reason: "task 102 consumes tasks 101 and 103", RequestedBy: "planner"}
	// The adapter's audit write must remain inside the task transaction. A
	// failure there rolls back edges, task/graph versions and the receipt.
	_, err = store.db.ExecContext(ctx, `CREATE TRIGGER fail_predecessor_audit BEFORE INSERT ON operations BEGIN SELECT RAISE(ABORT,'injected audit failure'); END`)
	require(t, err)
	if failed, err := commands.AddNativeTaskPredecessors(ctx, c, input); err == nil || failed.Status != "" {
		t.Fatalf("audit failure published an addition receipt: %+v / %v", failed, err)
	}
	before, err := store.TaskGraphSnapshot(ctx, c.Identity())
	require(t, err)
	if before.Authority.GraphRevision != 1 || before.ByID["102"].Version != 1 || len(before.Edges) != 0 {
		t.Fatalf("audit failure left partial task changes: %+v", before)
	}
	if _, found, err := store.LookupCommand(ctx, c.Identity(), input.RequestID); err != nil || found {
		t.Fatalf("audit failure left a command receipt: found=%v err=%v", found, err)
	}
	_, err = store.db.ExecContext(ctx, `DROP TRIGGER fail_predecessor_audit`)
	require(t, err)
	receipt, err := commands.AddNativeTaskPredecessors(ctx, c, input)
	require(t, err)
	if receipt.GraphRevision != 2 || receipt.TaskVersion != 2 || receipt.Status != "open" {
		t.Fatalf("unexpected addition receipt: %+v", receipt)
	}
	replayed, err := commands.AddNativeTaskPredecessors(ctx, c, input)
	require(t, err)
	if replayed.GraphRevision != receipt.GraphRevision || !slices.Equal(replayed.PredecessorIDs, receipt.PredecessorIDs) {
		t.Fatalf("addition was not idempotent: %+v / %+v", replayed, receipt)
	}
	page, err := (productquery.NewService(store, c.Identity())).ShowNativeTask(ctx, "102")
	require(t, err)
	if page.Tasks[0].DependencyReady || len(page.Tasks[0].UnmetPredecessors) != 2 || page.Tasks[0].UnmetPredecessors[0].TaskID != "101" {
		t.Fatalf("new predecessor did not block the existing task: %+v", page.Tasks[0])
	}
	bad := input
	bad.RequestID, bad.ExpectedGraphRevision, bad.ExpectedTaskVersion = newID(), 2, 2
	if _, err := commands.AddNativeTaskPredecessors(ctx, c, bad); err == nil || !strings.Contains(err.Error(), "duplicate_dependency") {
		t.Fatalf("duplicate edge accepted: %v", err)
	}
	bad.TaskID, bad.PredecessorIDs, bad.RequestID, bad.ExpectedTaskVersion = "101", []string{"102"}, newID(), 1
	if _, err := commands.AddNativeTaskPredecessors(ctx, c, bad); err == nil || !strings.Contains(err.Error(), "dependency_cycle") {
		t.Fatalf("cycle accepted: %v", err)
	}
	bad.TaskID, bad.PredecessorIDs, bad.RequestID = "103", []string{"101"}, newID()
	bad.ExpectedGraphRevision = 1
	if _, err := commands.AddNativeTaskPredecessors(ctx, c, bad); err == nil || !strings.Contains(err.Error(), "stale_version") {
		t.Fatalf("stale graph revision accepted: %v", err)
	}
	graph, err := (productquery.NewService(store, c.Identity())).TaskGraph(ctx, "", 100)
	require(t, err)
	if graph.GraphRevision != 2 || len(graph.Edges) != 2 {
		t.Fatalf("invalid additions wrote partial graph: %+v", graph)
	}
	reopened, err := openSQLiteFixture(filepath.Join(c.RunDir, "steerline.db"))
	require(t, err)
	defer reopened.Close()
	page, err = (productquery.NewService(reopened, c.Identity())).ShowNativeTask(ctx, "102")
	require(t, err)
	if page.GraphRevision != 2 || page.Tasks[0].DependencyReady || page.Tasks[0].Version != 2 {
		t.Fatalf("reopened database lost new dependency: %+v", page)
	}
}
