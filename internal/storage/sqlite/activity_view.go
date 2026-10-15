package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	p "github.com/erranliu/SteerLine/internal/storage/ports"
	"sort"
)

func (s Reader) ReadActivityView(ctx context.Context, project, candidate, run, attempt string, after int64, limit, budget int) (p.ActivityBinding, []p.DomainEvent, error) {
	return s.runnerActivity(ctx, project, candidate, run, attempt, after, limit, budget, false)
}
func (s Reader) ReadRecentActivityView(ctx context.Context, project, candidate, run, attempt string) (p.ActivityBinding, []p.DomainEvent, error) {
	return s.runnerActivity(ctx, project, candidate, run, attempt, 0, 20, 1<<20, true)
}
func (s Reader) runnerActivity(ctx context.Context, project, candidate, run, attempt string, after int64, limit, budget int, recent bool) (p.ActivityBinding, []p.DomainEvent, error) {
	binding := p.ActivityBinding{ProjectIdentity: project}
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return binding, nil, e
	}
	defer tx.Rollback()
	c, e := readRunnerCandidate(ctx, tx, project, candidate)
	if e != nil {
		return binding, nil, e
	}
	if c.RunID != run || attempt == "" {
		return binding, nil, errors.New("activity differs from original candidate Run")
	}
	// A past Attempt remains readable only with an original public event as proof.
	known := c.Projection != nil && c.Projection.Execution.AttemptID == attempt
	if !known {
		var n int
		e = tx.QueryRowContext(ctx, `SELECT count(*) FROM runner_events WHERE project_identity=? AND candidate_id=? AND json_extract(payload,'$.attempt_id')=?`, project, candidate, attempt).Scan(&n)
		if e != nil {
			return binding, nil, e
		}
		known = n > 0
	}
	if !known {
		return binding, nil, errors.New("activity Attempt is not recorded by Runner")
	}
	binding.ExecutionID = run
	binding.ExecutionWorkflowRunID = candidate
	binding.AttemptID = attempt
	binding.AttemptExecutionID = run
	limit = max(1, min(limit, 1000))
	order := " ASC"
	if recent {
		order = " DESC"
	}
	sqlQuery := `SELECT seq,payload FROM runner_events WHERE project_identity=? AND candidate_id=? AND seq>? AND json_extract(payload,'$.attempt_id')=? ORDER BY seq` + order + ` LIMIT ?`
	if recent {
		// Preserve readable changes even when technical events are much noisier.
		sqlQuery = `WITH ranked AS (SELECT seq,payload, row_number() OVER (PARTITION BY (json_extract(payload,'$.transition') IS NOT NULL) ORDER BY seq DESC) n FROM runner_events WHERE project_identity=? AND candidate_id=? AND seq>? AND json_extract(payload,'$.attempt_id')=?) SELECT seq,payload FROM ranked WHERE n<=CASE WHEN json_extract(payload,'$.transition') IS NOT NULL THEN 5 ELSE 10 END ORDER BY seq DESC LIMIT ?`
	}
	rows, e := tx.QueryContext(ctx, sqlQuery, project, candidate, after, attempt, limit)
	if e != nil {
		return binding, nil, e
	}
	events := []p.DomainEvent{}
	used := 0
	for rows.Next() {
		var seq int64
		var raw []byte
		if e = rows.Scan(&seq, &raw); e != nil {
			break
		}
		var event runnercontract.Event
		if e = json.Unmarshal(raw, &event); e != nil {
			break
		}
		if event.RunID != run {
			e = errors.New("Runner event Run identity differs")
			break
		}
		v := api.ActivityEvent{V: 1, Seq: uint64(seq), TS: event.CreatedAt, Source: runnercontract.Protocol, RunID: candidate, TaskID: c.TaskID, ItemID: event.EventID, Kind: "runner_event", Status: event.Type, Title: event.Type}
		if event.Transition != nil {
			v = runnerWorkflowActivity(v, *event.Transition)
		}
		payload, _ := json.Marshal(v)
		if used+len(payload) > budget {
			break
		}
		used += len(payload)
		events = append(events, p.DomainEvent{EventID: seq, ProjectIdentity: project, TaskID: c.TaskID, WorkflowRunID: candidate, ExecutionID: run, AttemptID: attempt, Type: "activity.runner_event", ReceivedAt: event.CreatedAt, Payload: payload, PayloadBytes: len(payload)})
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return binding, nil, e
	}
	if recent {
		sort.Slice(events, func(i, j int) bool { return events[i].EventID < events[j].EventID })
	}
	return binding, events, tx.Commit()
}
