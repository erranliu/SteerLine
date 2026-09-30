package query

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/task"
	"github.com/erranliu/SteerLine/internal/steerline/taskview"
)

const NativeTaskProtocol = "native-tasks/v1"

type TaskCursor struct {
	Version          int    `json:"v"`
	ProjectID        string `json:"p"`
	AuthorityVersion uint64 `json:"a"`
	GraphRevision    uint64 `json:"g"`
	DatabaseRevision uint64 `json:"d"`
	DatabaseID       string `json:"i"`
	Offset           int    `json:"o"`
}

func nativeErr(code, message string) error { return &task.Error{Code: code, Message: message} }

func (q Service) ListNativeTasks(ctx context.Context, cursor string, limit int) (contracts.NativeTaskPage, error) {
	if q.Tasks == nil || q.Project == "" {
		return contracts.NativeTaskPage{}, errors.New("query service store and project identity are required")
	}
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	offset := 0
	expectedDB := uint64(0)
	expectedDatabaseID := ""
	expectedAuthority, expectedGraph := uint64(0), uint64(0)
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return contracts.NativeTaskPage{}, nativeErr("cursor_invalidated", "cursor encoding is invalid")
		}
		var c TaskCursor
		if contracts.DecodeJSON(raw, &c) != nil || c.Version != 1 || c.ProjectID != q.Project || c.Offset < 0 || c.DatabaseRevision == 0 || len(c.DatabaseID) != 32 {
			return contracts.NativeTaskPage{}, nativeErr("cursor_invalidated", "cursor identity or sampling revision is invalid")
		}
		offset, expectedDB = c.Offset, c.DatabaseRevision
		expectedDatabaseID = c.DatabaseID
		expectedAuthority, expectedGraph = c.AuthorityVersion, c.GraphRevision
	}
	page, err := q.Tasks.NativeTaskPage(ctx, q.Project, "", expectedDatabaseID, expectedDB, offset, limit)
	if err != nil {
		return contracts.NativeTaskPage{}, err
	}
	if page.ProjectID != q.Project {
		return contracts.NativeTaskPage{}, nativeErr("authority_mismatch", "task page belongs to another project")
	}
	if expectedDB != 0 && (page.DatabaseID != expectedDatabaseID || page.DatabaseRevision != expectedDB || page.AuthorityVersion != expectedAuthority || page.GraphRevision != expectedGraph) {
		return contracts.NativeTaskPage{}, nativeErr("cursor_invalidated", "database or project authority changed between pages")
	}
	SetTaskPageCursor(&page, offset)
	return page, nil
}

func SetTaskPageCursor(page *contracts.NativeTaskPage, offset int) {
	if page.HasMore {
		c := TaskCursor{Version: 1, ProjectID: page.ProjectID, AuthorityVersion: page.AuthorityVersion, GraphRevision: page.GraphRevision, DatabaseID: page.DatabaseID, DatabaseRevision: page.DatabaseRevision, Offset: offset + len(page.Tasks)}
		b, _ := json.Marshal(c)
		page.NextCursor = base64.RawURLEncoding.EncodeToString(b)
	}
}

func (q Service) ShowNativeTask(ctx context.Context, id string) (contracts.NativeTaskPage, error) {
	if q.Tasks == nil || q.Project == "" {
		return contracts.NativeTaskPage{}, errors.New("query service store and project identity are required")
	}
	if err := task.ValidateID(id); err != nil {
		return contracts.NativeTaskPage{}, err
	}
	page, err := q.Tasks.NativeTaskPage(ctx, q.Project, id, "", 0, 0, 1)
	if err != nil {
		return contracts.NativeTaskPage{}, err
	}
	if page.ProjectID != q.Project {
		return contracts.NativeTaskPage{}, nativeErr("authority_mismatch", "task detail belongs to another project")
	}
	if len(page.Tasks) == 0 {
		return contracts.NativeTaskPage{}, nativeErr("task_not_found", "native task was not found")
	}
	if len(page.Tasks) != 1 || page.Tasks[0].TaskID != id {
		return contracts.NativeTaskPage{}, nativeErr("contract_mismatch", "task detail differs from the requested task")
	}
	return page, nil
}

func (q Service) TaskGraph(ctx context.Context, cursor string, limit int) (taskview.Page, error) {
	if q.Tasks == nil || q.Project == "" {
		return taskview.Page{}, errors.New("query service store and project identity are required")
	}
	request, err := taskview.NormalizePage(q.Project, cursor, limit)
	if err != nil {
		return taskview.Page{}, err
	}
	snapshot, err := q.readTaskGraph(ctx)
	if err != nil {
		return taskview.Page{}, err
	}
	return taskview.Paginate(snapshot, request)
}

func (q Service) QueryTaskGraph(ctx context.Context, filter taskview.Filter) (taskview.Page, error) {
	if q.Tasks == nil || q.Project == "" {
		return taskview.Page{}, errors.New("query service store and project identity are required")
	}
	filter, err := taskview.NormalizeFilter(q.Project, filter)
	if err != nil {
		return taskview.Page{}, err
	}
	snapshot, err := q.readTaskGraph(ctx)
	if err != nil {
		return taskview.Page{}, err
	}
	return taskview.Project(snapshot, filter)
}

func (q Service) ReadyNativeTaskSnapshot(ctx context.Context) (contracts.NativeReadyTaskSnapshot, error) {
	if q.Tasks == nil || q.Project == "" {
		return contracts.NativeReadyTaskSnapshot{}, errors.New("query service store and project identity are required")
	}
	graph, err := q.readTaskGraph(ctx)
	if err != nil {
		return contracts.NativeReadyTaskSnapshot{}, err
	}
	return ReadySnapshot(graph), nil
}

func (q Service) readTaskGraph(ctx context.Context) (task.Snapshot, error) {
	snapshot, err := q.Tasks.TaskGraphSnapshot(ctx, q.Project)
	if err != nil {
		return task.Snapshot{}, err
	}
	if snapshot.Authority.ProjectIdentity != q.Project {
		return task.Snapshot{}, nativeErr("authority_mismatch", "task graph belongs to another project")
	}
	return snapshot, nil
}

func ReadySnapshot(graph task.Snapshot) contracts.NativeReadyTaskSnapshot {
	return contracts.NativeReadyTaskSnapshot{ProjectID: graph.Authority.ProjectIdentity, AuthorityVersion: graph.Authority.AuthorityVersion, GraphRevision: graph.Authority.GraphRevision, DatabaseID: graph.DatabaseID, DatabaseRevision: graph.DatabaseRevision, SampledAt: graph.SampledAt, Tasks: task.ReadyTasks(graph)}
}

// ReadyNativeTasks preserves the candidate API used by dispatch and control.
func (q Service) ReadyNativeTasks(ctx context.Context) ([]contracts.TaskRecord, error) {
	snapshot, err := q.ReadyNativeTaskSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	return snapshot.Tasks, nil
}
