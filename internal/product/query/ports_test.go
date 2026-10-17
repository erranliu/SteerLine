package query

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// This reader has task capabilities only: no SQLite, command or event methods.
type taskReader struct {
	page  storageports.NativeTaskPage
	graph storageports.TaskGraphSnapshot
	reads int
}

func (r *taskReader) NativeTaskPage(context.Context, string, string, string, uint64, int, int) (storageports.NativeTaskPage, error) {
	r.reads++
	return r.page, nil
}

func (r *taskReader) TaskGraphSnapshot(context.Context, string) (storageports.TaskGraphSnapshot, error) {
	r.reads++
	return r.graph, nil
}

func TestTaskQueriesUseOnlyTaskReader(t *testing.T) {
	first := taskmodel.TaskRecord{TaskID: "1", Status: "open"}
	second := taskmodel.TaskRecord{TaskID: "2", Status: "open"}
	reader := &taskReader{
		page:  storageports.NativeTaskPage{Protocol: NativeTaskProtocol, ProjectID: "project", Tasks: []storageports.NativeTaskDetail{{TaskRecord: first}}},
		graph: storageports.TaskGraphSnapshot{Authority: taskmodel.ProjectTaskAuthority{ProjectIdentity: "project"}, Tasks: []taskmodel.TaskRecord{first, second}, ByID: map[string]taskmodel.TaskRecord{"1": first, "2": second}, Edges: []taskmodel.TaskDependencyRecord{{PredecessorID: "1", SuccessorID: "2"}}},
	}
	service := Service{Project: "project", Tasks: reader}
	page, err := service.ListNativeTasks(context.Background(), "", 1)
	if err != nil || len(page.Tasks) != 1 {
		t.Fatalf("task-only read port was rejected: %+v %v", page, err)
	}
	ready, err := service.ReadyNativeTasks(context.Background())
	if err != nil || len(ready) != 1 || ready[0].TaskID != "1" || reader.reads != 2 {
		t.Fatalf("readiness required another capability or ignored dependencies: %+v %v", ready, err)
	}
	if _, err := service.ReadProjectView(context.Background(), "", 1); err == nil {
		t.Fatal("missing project capability was accepted")
	}
	if _, err := service.ReadEvents(context.Background(), EventFilter{}, "", 1); err == nil {
		t.Fatal("missing event capability was accepted")
	}
}

func TestReadyQueriesRejectMissingCapabilityAndProject(t *testing.T) {
	reader := &taskReader{}
	for _, service := range []Service{{Project: "project"}, {Tasks: reader}} {
		if _, err := service.ReadyNativeTaskSnapshot(context.Background()); err == nil {
			t.Fatal("ready snapshot accepted a missing capability or project")
		}
		if _, err := service.ReadyNativeTasks(context.Background()); err == nil {
			t.Fatal("ready list accepted a missing capability or project")
		}
	}
	if reader.reads != 0 {
		t.Fatal("incomplete query configuration reached storage")
	}
}

func TestTaskCursorRemainsStrictWithoutSQLite(t *testing.T) {
	reader := &taskReader{}
	service := Service{Project: "project", Tasks: reader}
	input, err := json.Marshal(map[string]any{"v": 1, "p": "project", "i": strings.Repeat("a", 32), "d": 1, "o": 0, "unknown": true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ListNativeTasks(context.Background(), base64.RawURLEncoding.EncodeToString(input), 1)
	var coded *taskmodel.Error
	if !errors.As(err, &coded) || coded.Code != "cursor_invalidated" || reader.reads != 0 {
		t.Fatalf("unknown cursor field reached storage: reads=%d err=%v", reader.reads, err)
	}
}

func TestTaskQueriesRejectReaderScopeAndCursorDrift(t *testing.T) {
	reader := &taskReader{page: storageports.NativeTaskPage{ProjectID: "project", DatabaseID: strings.Repeat("a", 32), DatabaseRevision: 8, AuthorityVersion: 2, GraphRevision: 3, HasMore: true, Tasks: []storageports.NativeTaskDetail{{TaskRecord: taskmodel.TaskRecord{TaskID: "1"}}}}}
	service := Service{Project: "project", Tasks: reader}
	first, err := service.ListNativeTasks(context.Background(), "", 1)
	if err != nil || first.NextCursor == "" {
		t.Fatalf("initial page: %+v %v", first, err)
	}
	for _, changed := range []storageports.NativeTaskPage{
		{ProjectID: "project", DatabaseID: strings.Repeat("b", 32), DatabaseRevision: 8, AuthorityVersion: 2, GraphRevision: 3},
		{ProjectID: "project", DatabaseID: strings.Repeat("a", 32), DatabaseRevision: 9, AuthorityVersion: 2, GraphRevision: 3},
	} {
		reader.page = changed
		_, err := service.ListNativeTasks(context.Background(), first.NextCursor, 1)
		var coded *taskmodel.Error
		if !errors.As(err, &coded) || coded.Code != "cursor_invalidated" {
			t.Fatalf("different database sample was accepted: %+v %v", changed, err)
		}
	}
	reader.page.ProjectID = "other"
	if _, err := service.ListNativeTasks(context.Background(), "", 1); err == nil {
		t.Fatal("task page accepted another project")
	}
	reader.page = first
	if _, err := service.ShowNativeTask(context.Background(), "2"); err == nil {
		t.Fatal("task detail accepted another task")
	}
	reader.graph.Authority.ProjectIdentity = "other"
	if _, err := service.ReadyNativeTaskSnapshot(context.Background()); err == nil {
		t.Fatal("ready snapshot accepted another project")
	}
	if _, err := service.TaskGraph(context.Background(), "", 1); err == nil {
		t.Fatal("graph page accepted another project")
	}
}

type eventReader struct{ budgets []int }

func (r *eventReader) Events(_ context.Context, _, _, _ string, after int64, _ int, budget int) ([]storageports.DomainEvent, error) {
	r.budgets = append(r.budgets, budget)
	if after > 0 {
		return []storageports.DomainEvent{{EventID: 2, PayloadBytes: 3, PayloadOmitted: true}}, nil
	}
	return []storageports.DomainEvent{{EventID: 1, Payload: []byte("one"), PayloadBytes: 3}}, nil
}

func (r *eventReader) EventPayload(context.Context, string, int64, int64, int) ([]byte, int64, string, error) {
	return []byte("one"), 3, "digest", nil
}

func TestEventQueriesUseOnlyEventReader(t *testing.T) {
	reader := &eventReader{}
	service := Service{Project: "project", Events: reader}
	page, err := service.ReadEvents(context.Background(), EventFilter{ExecutionID: "execution"}, "", 1)
	if err != nil || len(page.Events) != 1 || !page.HasMore || page.NextCursor == "" || len(reader.budgets) != 2 || reader.budgets[0] != MaxEventPageBytes || reader.budgets[1] != 0 {
		t.Fatalf("event-only port lost the bounded lookahead: %+v budgets=%v err=%v", page, reader.budgets, err)
	}
	body, err := service.ReadEventBody(context.Background(), 1, 0, 10)
	if err != nil || !body.Complete || string(body.Data) != "one" {
		t.Fatalf("event body required another reader capability: %+v %v", body, err)
	}
}
