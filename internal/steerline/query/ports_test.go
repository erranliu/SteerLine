package query

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/task"
)

// This reader has task capabilities only: no SQLite, command or event methods.
type taskReader struct {
	page  contracts.NativeTaskPage
	graph task.Snapshot
	reads int
}

func (r *taskReader) NativeTaskPage(context.Context, string, string, string, uint64, int, int) (contracts.NativeTaskPage, error) {
	r.reads++
	return r.page, nil
}

func (r *taskReader) TaskGraphSnapshot(context.Context, string) (task.Snapshot, error) {
	r.reads++
	return r.graph, nil
}

func TestTaskQueriesUseOnlyTaskReader(t *testing.T) {
	first := task.TaskRecord{TaskID: "1", Status: "open"}
	second := task.TaskRecord{TaskID: "2", Status: "open"}
	reader := &taskReader{
		page:  contracts.NativeTaskPage{Protocol: NativeTaskProtocol, ProjectID: "project", Tasks: []contracts.NativeTaskDetail{{TaskRecord: first}}},
		graph: task.Snapshot{Authority: task.ProjectTaskAuthority{ProjectIdentity: "project"}, Tasks: []task.TaskRecord{first, second}, ByID: map[string]task.TaskRecord{"1": first, "2": second}, Edges: []task.TaskDependencyRecord{{PredecessorID: "1", SuccessorID: "2"}}},
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

func TestTaskCursorRemainsStrictWithoutSQLite(t *testing.T) {
	reader := &taskReader{}
	service := Service{Project: "project", Tasks: reader}
	input, err := json.Marshal(map[string]any{"v": 1, "p": "project", "i": strings.Repeat("a", 32), "d": 1, "o": 0, "unknown": true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ListNativeTasks(context.Background(), base64.RawURLEncoding.EncodeToString(input), 1)
	var coded *task.Error
	if !errors.As(err, &coded) || coded.Code != "cursor_invalidated" || reader.reads != 0 {
		t.Fatalf("unknown cursor field reached storage: reads=%d err=%v", reader.reads, err)
	}
}

type eventReader struct{ budgets []int }

func (r *eventReader) Events(_ context.Context, _, _, _ string, after int64, _ int, budget int) ([]contracts.DomainEvent, error) {
	r.budgets = append(r.budgets, budget)
	if after > 0 {
		return []contracts.DomainEvent{{EventID: 2, PayloadBytes: 3, PayloadOmitted: true}}, nil
	}
	return []contracts.DomainEvent{{EventID: 1, Payload: []byte("one"), PayloadBytes: 3}}, nil
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
