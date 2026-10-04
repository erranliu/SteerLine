package steerline

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tasklegacy "github.com/erranliu/SteerLine/internal/steerline/task"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestAddNativeTaskPredecessorsIsAtomicIdempotentAndBlocksClaim(t *testing.T) {
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	commands := CommandService{Store: store, Project: c.Identity()}
	batch := taskBatchFixture(t, c, commit, batchTasks(101, 102, 103), nil)
	_, err := commands.CreateTaskBatch(ctx, c, batch, newID(), 1, "planner")
	require(t, err)
	input := TaskPredecessorAddInput{TaskID: "102", PredecessorIDs: []string{"103", "101"}, RequestID: newID(), ExpectedTaskVersion: 1, ExpectedGraphRevision: 1, Reason: "task 102 consumes tasks 101 and 103", RequestedBy: "planner"}
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
	page, err := (NewQueryService(store, c.Identity())).ShowNativeTask(ctx, "102")
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
	graph, err := (NewQueryService(store, c.Identity())).TaskGraph(ctx, "", 100)
	require(t, err)
	if graph.GraphRevision != 2 || len(graph.Edges) != 2 {
		t.Fatalf("invalid additions wrote partial graph: %+v", graph)
	}
	reopened, err := openSQLiteFixture(filepath.Join(c.RunDir, "steerline.db"))
	require(t, err)
	defer reopened.Close()
	page, err = (NewQueryService(reopened, c.Identity())).ShowNativeTask(ctx, "102")
	require(t, err)
	if page.GraphRevision != 2 || page.Tasks[0].DependencyReady || page.Tasks[0].Version != 2 {
		t.Fatalf("reopened database lost new dependency: %+v", page)
	}
}

func TestNativePredecessorAdditionRejectsDeliveredAndDiscardedSuccessor(t *testing.T) {
	graph := taskGraphSnapshot{Authority: tasklegacy.ProjectTaskAuthority{ProjectIdentity: "fixture"}, ByID: map[string]taskmodel.TaskRecord{"1": {TaskID: "1", Status: "open"}, "2": {TaskID: "2", Status: "closed"}}, Deliveries: map[string]TaskDeliveryRecord{"2": {TaskID: "2"}}}
	if err := validateNativePredecessorAddition(graph, "2", []string{"1"}); err == nil || !strings.Contains(err.Error(), "already_merged") {
		t.Fatalf("delivered successor accepted: %v", err)
	}
	graph.ByID["2"] = taskmodel.TaskRecord{TaskID: "2", Status: "discarded"}
	delete(graph.Deliveries, "2")
	if err := validateNativePredecessorAddition(graph, "2", []string{"1"}); err == nil || !strings.Contains(err.Error(), "task_discarded") {
		t.Fatalf("discarded successor accepted: %v", err)
	}
}
