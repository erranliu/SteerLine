package taskview

import (
	"strings"
	"testing"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestGraphViewUsesOneSnapshot(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	records := []taskmodel.TaskRecord{
		{TaskID: "1", Title: "merged", Status: "closed"},
		{TaskID: "2", Title: "ready", Status: "open"},
		{TaskID: "3", Title: "other", Status: "open"},
	}
	snapshot := storageports.TaskGraphSnapshot{
		Authority:  executionmodel.ProjectTaskAuthority{AuthorityVersion: 2, GraphRevision: 3},
		DatabaseID: strings.Repeat("a", 32), DatabaseRevision: 4, SampledAt: at,
		Tasks: records, ByID: map[string]taskmodel.TaskRecord{"1": records[0], "2": records[1], "3": records[2]},
		Edges:      []taskmodel.TaskDependencyRecord{{PredecessorID: "1", SuccessorID: "2"}},
		Deliveries: map[string]executionmodel.TaskDeliveryRecord{"1": {TaskID: "1"}},
	}
	filter, err := NormalizeFilter("project", Filter{CenterTaskID: "1", Direction: "downstream", Depth: 1})
	if err != nil {
		t.Fatal(err)
	}
	view, err := Project(snapshot, filter)
	if err != nil || len(view.Nodes) != 2 || view.Nodes[1].TaskID != "2" || !view.Nodes[1].DependencyReady {
		t.Fatalf("projected graph = %+v, err = %v", view, err)
	}
	request, err := NormalizePage("project", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Paginate(snapshot, request)
	if err != nil || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page = %+v, err = %v", first, err)
	}
	request, err = NormalizePage("project", first.NextCursor, 1)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.DatabaseRevision++
	if _, err = Paginate(snapshot, request); err == nil || !strings.Contains(err.Error(), "cursor_invalidated") {
		t.Fatalf("changed snapshot should invalidate cursor: %v", err)
	}
}
