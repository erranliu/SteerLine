package query

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

const (
	DefaultEventPageSize = 200
	MaxEventPageSize     = 1000
	MaxEventPageBytes    = 1 << 20
	// Bound transient payload allocation as well as the returned page size.
	EventReadBatchSize = 32
)

type Service struct {
	Project   string
	Workflows storageports.WorkflowReader
	Events    storageports.EventReader
	Tasks     storageports.TaskReader
	Projects  storageports.ProjectReader
	Activity  storageports.ActivityReader
}

type EventFilter struct {
	ExecutionID string
	AttemptID   string
}

type EventPage struct {
	Events       []storageports.DomainEvent `json:"events"`
	NextCursor   string                     `json:"next_cursor,omitempty"`
	HasMore      bool                       `json:"has_more"`
	PayloadBytes int                        `json:"payload_bytes"`
}

type EventBodyPage struct {
	EventID    int64  `json:"event_id"`
	Offset     int64  `json:"offset"`
	NextOffset int64  `json:"next_offset"`
	TotalBytes int64  `json:"total_bytes"`
	Digest     string `json:"digest"`
	Complete   bool   `json:"complete"`
	Data       []byte `json:"data"`
}

type queryCursor struct {
	Version   int    `json:"v"`
	Project   string `json:"p"`
	Execution string `json:"e,omitempty"`
	Attempt   string `json:"a,omitempty"`
	After     int64  `json:"n"`
}

type CursorError struct{ Code string }

func (e *CursorError) Error() string { return e.Code }

func (q Service) Workflow(ctx context.Context, workflowRunID string) (storageports.ExecutionSnapshot, error) {
	if q.Workflows == nil || q.Project == "" {
		return storageports.ExecutionSnapshot{}, errors.New("query service store and project identity are required")
	}
	snapshot, err := q.Workflows.Query(ctx, q.Project)
	if err != nil {
		return storageports.ExecutionSnapshot{}, err
	}
	filtered := storageports.ExecutionSnapshot{ProjectIdentity: snapshot.ProjectIdentity, WorkflowRuns: []executionmodel.WorkflowRunRecord{}, Executions: []executionmodel.ExecutionRecord{}, Attempts: []executionmodel.StepAttemptRecord{}, Decisions: []executionmodel.DecisionRecord{}}
	for _, run := range snapshot.WorkflowRuns {
		if workflowRunID == "" || run.WorkflowRunID == workflowRunID {
			filtered.WorkflowRuns = append(filtered.WorkflowRuns, run)
		}
	}
	if workflowRunID != "" && len(filtered.WorkflowRuns) == 0 {
		return filtered, fmt.Errorf("workflow run %s not found", workflowRunID)
	}
	executionIDs := map[string]bool{}
	for _, execution := range snapshot.Executions {
		if workflowRunID == "" || execution.WorkflowRunID == workflowRunID {
			filtered.Executions = append(filtered.Executions, execution)
			executionIDs[execution.ExecutionID] = true
		}
	}
	for _, attempt := range snapshot.Attempts {
		if executionIDs[attempt.ExecutionID] {
			filtered.Attempts = append(filtered.Attempts, attempt)
		}
	}
	for _, decision := range snapshot.Decisions {
		if workflowRunID == "" || decision.WorkflowRunID == workflowRunID {
			filtered.Decisions = append(filtered.Decisions, decision)
		}
	}
	return filtered, nil
}

func (q Service) ReadEvents(ctx context.Context, filter EventFilter, cursor string, limit int) (EventPage, error) {
	if q.Events == nil || q.Project == "" {
		return EventPage{}, errors.New("query service store and project identity are required")
	}
	if limit < 1 {
		limit = DefaultEventPageSize
	}
	if limit > MaxEventPageSize {
		limit = MaxEventPageSize
	}
	after := int64(0)
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return EventPage{}, &CursorError{Code: "cursor_invalidated"}
		}
		var c queryCursor
		if json.Unmarshal(raw, &c) != nil || c.Version != 1 || c.Project != q.Project || c.Execution != filter.ExecutionID || c.Attempt != filter.AttemptID || c.After < 0 {
			return EventPage{}, &CursorError{Code: "cursor_invalidated"}
		}
		after = c.After
	}
	page := EventPage{Events: []storageports.DomainEvent{}}
	for len(page.Events) < limit {
		batchSize := min(EventReadBatchSize, limit-len(page.Events))
		events, err := q.Events.Events(ctx, q.Project, filter.ExecutionID, filter.AttemptID, after, batchSize, MaxEventPageBytes-page.PayloadBytes)
		if err != nil {
			return EventPage{}, err
		}
		if len(events) == 0 {
			break
		}
		for _, ev := range events {
			if ev.PayloadBytes > MaxEventPageBytes {
				ev.Payload = nil
				ev.PayloadOmitted = true
			}
			if ev.PayloadBytes <= MaxEventPageBytes && page.PayloadBytes+ev.PayloadBytes > MaxEventPageBytes {
				page.HasMore = true
				break
			}
			page.Events = append(page.Events, ev)
			page.PayloadBytes += len(ev.Payload)
			after = ev.EventID
		}
		if page.HasMore || len(events) < batchSize {
			break
		}
	}
	if len(page.Events) > 0 {
		if !page.HasMore && len(page.Events) == limit {
			more, err := q.Events.Events(ctx, q.Project, filter.ExecutionID, filter.AttemptID, after, 1, 0)
			if err != nil {
				return EventPage{}, err
			}
			page.HasMore = len(more) > 0
		}
		if page.HasMore {
			last := page.Events[len(page.Events)-1].EventID
			c := queryCursor{Version: 1, Project: q.Project, Execution: filter.ExecutionID, Attempt: filter.AttemptID, After: last}
			b, _ := json.Marshal(c)
			page.NextCursor = base64.RawURLEncoding.EncodeToString(b)
		}
	}
	return page, nil
}

func (q Service) ReadEventBody(ctx context.Context, eventID, offset int64, limit int) (EventBodyPage, error) {
	if q.Events == nil || q.Project == "" {
		return EventBodyPage{}, errors.New("query service store and project identity are required")
	}
	if offset < 0 {
		return EventBodyPage{}, &CursorError{Code: "cursor_invalidated"}
	}
	if limit < 1 {
		limit = 256 << 10
	}
	if limit > 256<<10 {
		limit = 256 << 10
	}
	data, total, digest, err := q.Events.EventPayload(ctx, q.Project, eventID, offset, limit)
	if err != nil {
		return EventBodyPage{}, err
	}
	next := offset + int64(len(data))
	return EventBodyPage{EventID: eventID, Offset: offset, NextOffset: next, TotalBytes: total, Digest: digest, Complete: next >= total, Data: data}, nil
}
