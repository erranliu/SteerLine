package host

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	productquery "github.com/erranliu/SteerLine/internal/product/query"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestPS3TaskGraphPaginationAndDirectRelations(t *testing.T) {
	c, commit, store := openDAGFixture(t)
	ctx := context.Background()
	edges := []taskmodel.TaskDependencyRecord{{ProjectIdentity: c.Identity(), PredecessorID: "1", SuccessorID: "2"}, {ProjectIdentity: c.Identity(), PredecessorID: "2", SuccessorID: "3"}}
	seedNativeGraphForView(t, c, commit, store, 4, edges)
	q := productquery.NewService(store, c.Identity())
	page, err := q.TaskGraph(ctx, "", 2)
	require(t, err)
	if page.Protocol != "native-task-graph/v2" || len(page.Nodes) != 2 || len(page.Edges) != 1 || !page.HasMore || !page.Boundary.Truncated || len(page.Boundary.Edges) != 1 || page.Boundary.Edges[0].Successor != "3" || page.SampledAt.IsZero() {
		t.Fatalf("bounded page: %+v", page)
	}
	detail, err := q.ShowNativeTask(ctx, "2")
	require(t, err)
	if len(detail.Tasks) != 1 || len(detail.Tasks[0].Predecessors) != 1 || len(detail.Tasks[0].Successors) != 1 || detail.Tasks[0].DependencyReady || detail.Tasks[0].Contract.SpecRef.Path == "" {
		t.Fatalf("basic relations lost: %+v", detail)
	}
	next, err := q.TaskGraph(ctx, page.NextCursor, 2)
	require(t, err)
	if len(next.Nodes) != 2 || next.HasMore || next.Nodes[0].TaskID != "3" || next.DatabaseID != page.DatabaseID || next.DatabaseRevision != page.DatabaseRevision {
		t.Fatalf("next page: %+v", next)
	}
	body, err := json.Marshal(next)
	require(t, err)
	if strings.Contains(string(body), "\u202e") || !strings.Contains(string(body), `\u202e`) {
		t.Fatal("terminal bidi output protection lost")
	}
	current := detail.Tasks[0].TaskRecord
	current.Version++
	current.Status = "in_progress"
	current.Claimant = "fixture"
	require(t, store.transactRecords(ctx, func(tx *fixtureTransaction) error { return tx.UpdateNativeTaskCAS(current, current.Version-1) }))
	if _, err := q.TaskGraph(ctx, page.NextCursor, 2); err == nil || !strings.Contains(err.Error(), "cursor_invalidated") {
		t.Fatalf("different database snapshot accepted: %v", err)
	}
}
