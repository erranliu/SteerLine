package query

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnercontract"
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
	return q.readRunnerActivity(ctx, run, execution, attempt, cursor, false)
}

func (q Service) ReadRecentActivity(ctx context.Context, run, execution, attempt string) (api.LogBatch, error) {
	return q.readRunnerActivity(ctx, run, execution, attempt, api.ActivityCursor{}, true)
}

func (q Service) readRunnerActivity(ctx context.Context, run, execution, attempt string, cursor api.ActivityCursor, recent bool) (batch api.LogBatch, err error) {
	if q.Activity == nil || q.Project == "" || run == "" || execution == "" || attempt == "" {
		return batch, errors.New("activity reader and exact project/run/execution/attempt are required")
	}
	limit := DefaultEventPageSize
	if recent {
		limit = 20
	}
	binding, records, err := q.Activity.OpenActivityView(ctx, q.Project, run, execution, attempt, cursor.AfterEventID, limit, recent)
	if err != nil {
		return batch, err
	}
	if records == nil {
		return batch, errors.New("activity read stream is unavailable")
	}
	defer func() {
		err = errors.Join(err, records.Close())
		if err != nil {
			batch = api.LogBatch{}
		}
	}()
	events := []ports.DomainEvent{}
	used := 0
	for {
		record, readErr := records.Next()
		if readErr != nil {
			return batch, readErr
		}
		if record == nil {
			break
		}
		event := record.Event
		if event.RunID != execution || event.AttemptID != attempt {
			return batch, errors.New("Runner activity differs from original Run/Attempt")
		}
		v := api.ActivityEvent{V: 1, Seq: uint64(record.EventID), TS: event.CreatedAt, Source: runnercontract.Protocol, RunID: run, TaskID: record.TaskID, ItemID: event.EventID, Kind: "runner_event", Status: event.Type, Title: event.Type}
		if event.Transition != nil {
			v = runnerWorkflowActivity(v, *event.Transition)
		}
		payload, marshalErr := json.Marshal(v)
		if marshalErr != nil {
			return batch, marshalErr
		}
		if used+len(payload) > MaxEventPageBytes {
			break
		}
		used += len(payload)
		events = append(events, ports.DomainEvent{EventID: record.EventID, ProjectIdentity: q.Project, TaskID: record.TaskID, WorkflowRunID: run, ExecutionID: execution, AttemptID: attempt, Type: "activity.runner_event", ReceivedAt: event.CreatedAt, Payload: payload, PayloadBytes: len(payload)})
	}
	if recent {
		sort.Slice(events, func(i, j int) bool { return events[i].EventID < events[j].EventID })
	}
	return ProjectActivity(q.Project, run, execution, attempt, cursor, binding, events)
}
