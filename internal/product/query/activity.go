package query

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// ProjectActivity consumes caller-collected canonical records. The reader must
// collect binding facts and events in one read transaction; native logs cannot
// fill a canonical gap and no file path is accepted here.
func ProjectActivity(project, run, execution, attempt string, cursor api.ActivityCursor, binding ports.ActivityBinding, events []ports.DomainEvent) (api.LogBatch, error) {
	if run == "" || execution == "" || attempt == "" {
		return api.LogBatch{}, errors.New("activity query identity is missing")
	}
	if cursor.Initialized && (cursor.RunID != run || cursor.ExecutionID != execution || cursor.AttemptID != attempt) {
		return api.LogBatch{}, errors.New("activity cursor belongs to another execution or attempt")
	}
	if binding.ProjectIdentity != project {
		return api.LogBatch{}, errors.New("activity binding belongs to another project")
	}
	if binding.ExecutionID != execution || binding.ExecutionWorkflowRunID != run {
		return api.LogBatch{}, errors.New("activity execution does not belong to the requested run")
	}
	if binding.AttemptID != attempt || binding.AttemptExecutionID != execution {
		return api.LogBatch{}, errors.New("activity attempt does not belong to the requested execution")
	}
	batch := api.LogBatch{Cursor: api.ActivityCursor{RunID: run, ExecutionID: execution, AttemptID: attempt, AfterEventID: cursor.AfterEventID, Initialized: true}}
	for _, e := range events {
		if e.ProjectIdentity != project || e.WorkflowRunID != run || e.ExecutionID != execution || e.AttemptID != attempt || e.EventID <= batch.Cursor.AfterEventID {
			return api.LogBatch{}, errors.New("canonical activity page changed its fixed identity or order")
		}
		batch.Cursor.AfterEventID = e.EventID
		if !strings.HasPrefix(e.Type, "activity.") {
			continue
		}
		var v api.ActivityEvent
		if len(e.Payload) == 0 || json.Unmarshal(e.Payload, &v) != nil {
			batch.Diagnostics = append(batch.Diagnostics, "规范活动尚未完整采集；原始文件仅供手动诊断")
			continue
		}
		if v.Kind == "worker_state" {
			var transition struct {
				From executionmodel.WorkerStateFields `json:"from"`
				To   executionmodel.WorkerStateFields `json:"to"`
			}
			raw, err := json.Marshal(v.Meta)
			if err != nil || json.Unmarshal(raw, &transition) != nil || transition.To.WorkflowStage == "" || transition.To.WorkflowStatus == "" {
				batch.Diagnostics = append(batch.Diagnostics, "规范流程变化字段损坏或缺少 workflow_stage/workflow_status；原始记录保留，不补画")
				continue
			}
		}
		v.Seq = uint64(e.EventID)
		batch.Activity = append(batch.Activity, v)
	}
	if !cursor.Initialized && len(batch.Activity) == 0 {
		batch.Diagnostics = append(batch.Diagnostics, "数据库中暂无已采集活动；不会从原始日志补画")
	}
	return batch, nil
}

// ReadActivity reads one fixed snapshot before projecting public activity values.
func (q Service) ReadActivity(ctx context.Context, run, execution, attempt string, cursor api.ActivityCursor) (api.LogBatch, error) {
	if q.Activity == nil || q.Project == "" {
		return api.LogBatch{}, errors.New("activity reader and project identity are required")
	}
	binding, events, err := q.Activity.ReadActivityView(ctx, q.Project, run, execution, attempt, cursor.AfterEventID, DefaultEventPageSize, MaxEventPageBytes)
	if err != nil {
		return api.LogBatch{}, err
	}
	return ProjectActivity(q.Project, run, execution, attempt, cursor, binding, events)
}

func (q Service) ReadRecentActivity(ctx context.Context, run, execution, attempt string) (api.LogBatch, error) {
	if q.Activity == nil || q.Project == "" || run == "" || execution == "" || attempt == "" {
		return api.LogBatch{}, errors.New("recent activity requires an exact project/run/execution/attempt")
	}
	binding, events, err := q.Activity.ReadRecentActivityView(ctx, q.Project, run, execution, attempt)
	if err != nil {
		return api.LogBatch{}, err
	}
	return ProjectActivity(q.Project, run, execution, attempt, api.ActivityCursor{}, binding, events)
}
