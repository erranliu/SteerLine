package query

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"

	api "github.com/erranliu/SteerLine/internal/product/api"
	taskview "github.com/erranliu/SteerLine/internal/product/query/taskgraph"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
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

func nativeErr(code, message string) error { return &taskmodel.Error{Code: code, Message: message} }

func (q Service) ListNativeTasks(ctx context.Context, cursor string, limit int) (storageports.NativeTaskPage, error) {
	if q.Tasks == nil || q.Project == "" {
		return storageports.NativeTaskPage{}, errors.New("query service store and project identity are required")
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
			return storageports.NativeTaskPage{}, nativeErr("cursor_invalidated", "cursor encoding is invalid")
		}
		var c TaskCursor
		if contracts.DecodeJSON(raw, &c) != nil || c.Version != 1 || c.ProjectID != q.Project || c.Offset < 0 || c.DatabaseRevision == 0 || len(c.DatabaseID) != 32 {
			return storageports.NativeTaskPage{}, nativeErr("cursor_invalidated", "cursor identity or sampling revision is invalid")
		}
		offset, expectedDB = c.Offset, c.DatabaseRevision
		expectedDatabaseID = c.DatabaseID
		expectedAuthority, expectedGraph = c.AuthorityVersion, c.GraphRevision
	}
	page, err := q.Tasks.NativeTaskPage(ctx, q.Project, "", expectedDatabaseID, expectedDB, offset, limit)
	if err != nil {
		return storageports.NativeTaskPage{}, err
	}
	if page.ProjectID != q.Project {
		return storageports.NativeTaskPage{}, nativeErr("authority_mismatch", "task page belongs to another project")
	}
	if expectedDB != 0 && (page.DatabaseID != expectedDatabaseID || page.DatabaseRevision != expectedDB || page.AuthorityVersion != expectedAuthority || page.GraphRevision != expectedGraph) {
		return storageports.NativeTaskPage{}, nativeErr("cursor_invalidated", "database or project authority changed between pages")
	}
	SetTaskPageCursor(&page, offset)
	return page, nil
}

func SetTaskPageCursor(page *storageports.NativeTaskPage, offset int) {
	if page.HasMore {
		c := TaskCursor{Version: 1, ProjectID: page.ProjectID, AuthorityVersion: page.AuthorityVersion, GraphRevision: page.GraphRevision, DatabaseID: page.DatabaseID, DatabaseRevision: page.DatabaseRevision, Offset: offset + len(page.Tasks)}
		b, _ := json.Marshal(c)
		page.NextCursor = base64.RawURLEncoding.EncodeToString(b)
	}
}

func (q Service) ShowNativeTask(ctx context.Context, id string) (storageports.NativeTaskPage, error) {
	if q.Tasks == nil || q.Project == "" {
		return storageports.NativeTaskPage{}, errors.New("query service store and project identity are required")
	}
	if err := taskmodel.ValidateID(id); err != nil {
		return storageports.NativeTaskPage{}, err
	}
	page, err := q.Tasks.NativeTaskPage(ctx, q.Project, id, "", 0, 0, 1)
	if err != nil {
		return storageports.NativeTaskPage{}, err
	}
	if page.ProjectID != q.Project {
		return storageports.NativeTaskPage{}, nativeErr("authority_mismatch", "task detail belongs to another project")
	}
	if len(page.Tasks) == 0 {
		return storageports.NativeTaskPage{}, nativeErr("task_not_found", "native task was not found")
	}
	if len(page.Tasks) != 1 || page.Tasks[0].TaskID != id {
		return storageports.NativeTaskPage{}, nativeErr("contract_mismatch", "task detail differs from the requested task")
	}
	return page, nil
}

func (q Service) TaskGraph(ctx context.Context, cursor string, limit int) (api.GraphPage, error) {
	if q.Tasks == nil || q.Project == "" {
		return api.GraphPage{}, errors.New("query service store and project identity are required")
	}
	request, err := taskview.NormalizePage(q.Project, cursor, limit)
	if err != nil {
		return api.GraphPage{}, err
	}
	snapshot, err := q.readTaskGraph(ctx)
	if err != nil {
		return api.GraphPage{}, err
	}
	page, err := taskview.Paginate(snapshot, request)
	if err != nil {
		return api.GraphPage{}, err
	}
	return TaskGraphPage(page), nil
}

func (q Service) QueryTaskGraph(ctx context.Context, filter api.GraphFilter) (api.GraphPage, error) {
	if q.Tasks == nil || q.Project == "" {
		return api.GraphPage{}, errors.New("query service store and project identity are required")
	}
	rawFilter := taskview.Filter{ProjectID: filter.ProjectID, Status: filter.Status, CenterTaskID: filter.CenterTaskID, Direction: filter.Direction, Depth: filter.Depth, Limit: filter.Limit}
	rawFilter, err := taskview.NormalizeFilter(q.Project, rawFilter)
	if err != nil {
		return api.GraphPage{}, err
	}
	snapshot, err := q.readTaskGraph(ctx)
	if err != nil {
		return api.GraphPage{}, err
	}
	page, err := taskview.Project(snapshot, rawFilter)
	if err != nil {
		return api.GraphPage{}, err
	}
	return TaskGraphPage(page), nil
}

func (q Service) ReadyNativeTaskSnapshot(ctx context.Context) (storageports.NativeReadyTaskSnapshot, error) {
	if q.Tasks == nil || q.Project == "" {
		return storageports.NativeReadyTaskSnapshot{}, errors.New("query service store and project identity are required")
	}
	graph, err := q.readTaskGraph(ctx)
	if err != nil {
		return storageports.NativeReadyTaskSnapshot{}, err
	}
	return ReadySnapshot(graph), nil
}

func (q Service) readTaskGraph(ctx context.Context) (storageports.TaskGraphSnapshot, error) {
	snapshot, err := q.Tasks.TaskGraphSnapshot(ctx, q.Project)
	if err != nil {
		return storageports.TaskGraphSnapshot{}, err
	}
	if snapshot.Authority.ProjectIdentity != q.Project {
		return storageports.TaskGraphSnapshot{}, nativeErr("authority_mismatch", "task graph belongs to another project")
	}
	return snapshot, nil
}

func ReadySnapshot(graph storageports.TaskGraphSnapshot) storageports.NativeReadyTaskSnapshot {
	return storageports.NativeReadyTaskSnapshot{ProjectID: graph.Authority.ProjectIdentity, AuthorityVersion: graph.Authority.AuthorityVersion, GraphRevision: graph.Authority.GraphRevision, DatabaseID: graph.DatabaseID, DatabaseRevision: graph.DatabaseRevision, SampledAt: graph.SampledAt, Tasks: taskmodel.ReadyTasks(graph.TaskGraph())}
}

// ReadyNativeTasks preserves the candidate API used by dispatch and control.
func (q Service) ReadyNativeTasks(ctx context.Context) ([]taskmodel.TaskRecord, error) {
	snapshot, err := q.ReadyNativeTaskSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	return snapshot.Tasks, nil
}
