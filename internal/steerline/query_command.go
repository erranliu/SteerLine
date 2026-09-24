package steerline

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	DefaultEventPageSize = 200
	MaxEventPageSize     = 1000
	MaxEventPageBytes    = 1 << 20
)

type QueryService struct {
	Store   DomainStore
	Project string
}

type EventFilter struct {
	ExecutionID string
	AttemptID   string
}
type EventPage struct {
	Events       []DomainEvent `json:"events"`
	NextCursor   string        `json:"next_cursor,omitempty"`
	HasMore      bool          `json:"has_more"`
	PayloadBytes int           `json:"payload_bytes"`
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

func (q QueryService) Workflow(ctx context.Context, workflowRunID string) (DomainSnapshot, error) {
	if q.Store == nil || q.Project == "" {
		return DomainSnapshot{}, errors.New("query service store and project identity are required")
	}
	snapshot, err := q.Store.Query(ctx, q.Project)
	if err != nil {
		return DomainSnapshot{}, err
	}
	filtered := DomainSnapshot{ProjectIdentity: snapshot.ProjectIdentity, Tasks: []TaskRecord{}, WorkflowRuns: []WorkflowRunRecord{}, Executions: []ExecutionRecord{}, Attempts: []StepAttemptRecord{}, Decisions: []DecisionRecord{}}
	taskIDs := map[string]bool{}
	for _, run := range snapshot.WorkflowRuns {
		if workflowRunID == "" || run.WorkflowRunID == workflowRunID {
			filtered.WorkflowRuns = append(filtered.WorkflowRuns, run)
			taskIDs[run.TaskID] = true
		}
	}
	if workflowRunID != "" && len(filtered.WorkflowRuns) == 0 {
		return filtered, fmt.Errorf("workflow run %s not found", workflowRunID)
	}
	for _, task := range snapshot.Tasks {
		if taskIDs[task.TaskID] {
			filtered.Tasks = append(filtered.Tasks, task)
		}
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

func (q QueryService) ReadEvents(ctx context.Context, filter EventFilter, cursor string, limit int) (EventPage, error) {
	if q.Store == nil || q.Project == "" {
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
	page := EventPage{Events: []DomainEvent{}}
	for len(page.Events) < limit {
		events, err := q.Store.Events(ctx, q.Project, filter.ExecutionID, filter.AttemptID, after, 1)
		if err != nil {
			return EventPage{}, err
		}
		if len(events) == 0 {
			break
		}
		ev := events[0]
		if ev.PayloadBytes > MaxEventPageBytes {
			ev.Payload = nil
			ev.PayloadOmitted = true
		}
		if !ev.PayloadOmitted && page.PayloadBytes+len(ev.Payload) > MaxEventPageBytes {
			page.HasMore = true
			break
		}
		page.Events = append(page.Events, ev)
		page.PayloadBytes += len(ev.Payload)
		after = ev.EventID
	}
	if len(page.Events) > 0 {
		if !page.HasMore && len(page.Events) == limit {
			more, err := q.Store.Events(ctx, q.Project, filter.ExecutionID, filter.AttemptID, after, 1)
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

func (q QueryService) ReadEventBody(ctx context.Context, eventID, offset int64, limit int) (EventBodyPage, error) {
	if q.Store == nil || q.Project == "" {
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
	data, total, digest, err := q.Store.EventPayload(ctx, q.Project, eventID, offset, limit)
	if err != nil {
		return EventBodyPage{}, err
	}
	next := offset + int64(len(data))
	return EventBodyPage{EventID: eventID, Offset: offset, NextOffset: next, TotalBytes: total, Digest: digest, Complete: next >= total, Data: data}, nil
}

type CommandService struct {
	Store   DomainStore
	Project string
}

func (c CommandService) Submit(ctx context.Context, command CommandRecord) (CommandRecord, bool, error) {
	if c.Store == nil || c.Project == "" {
		return command, false, errors.New("command service store and project identity are required")
	}
	command.ProjectIdentity = c.Project
	var result CommandRecord
	var created bool
	err := c.Store.Transact(ctx, func(tx DomainTx) error { var err error; result, created, err = tx.EnqueueCommand(command); return err })
	return result, created, err
}

func (c CommandService) AnswerDecision(ctx context.Context, decisionID string, expectedVersion uint64, idempotencyKey, requestedBy string, answer []byte) (CommandRecord, bool, error) {
	if decisionID == "" || expectedVersion == 0 || idempotencyKey == "" || requestedBy == "" {
		return CommandRecord{}, false, errors.New("decision answer requires decision ID, expected version, idempotency key and requester")
	}
	var value DecisionAnswer
	if err := decode(answer, &value); err != nil {
		return CommandRecord{}, false, err
	}
	value.OptionID = strings.TrimSpace(value.OptionID)
	value.Note = clip(strings.TrimSpace(value.Note), 4000)
	value.AnsweredBy = clip(strings.TrimSpace(value.AnsweredBy), 200)
	if value.DecisionID != decisionID || value.RunID == "" || value.AnsweredBy != requestedBy {
		return CommandRecord{}, false, errors.New("answer identity must match the command target and requester")
	}
	answer, err := json.Marshal(value)
	if err != nil {
		return CommandRecord{}, false, err
	}
	return c.Submit(ctx, CommandRecord{CommandID: newID(), Kind: "answer_decision", TargetID: decisionID, ExpectedVersion: expectedVersion, IdempotencyKey: idempotencyKey, RequestedBy: requestedBy, Payload: append([]byte(nil), answer...), Status: "pending", CreatedAt: time.Now().UTC()})
}

func (c CommandService) Pending(ctx context.Context, limit int) ([]CommandRecord, error) {
	if c.Store == nil || c.Project == "" {
		return nil, errors.New("command service store and project identity are required")
	}
	return c.Store.Commands(ctx, c.Project, "pending", limit)
}

func (c CommandService) Complete(ctx context.Context, command CommandRecord, result []byte) error {
	if c.Store == nil || c.Project == "" {
		return errors.New("command service store and project identity are required")
	}
	command.ProjectIdentity = c.Project
	command.Result = append([]byte(nil), result...)
	command.Status = "completed"
	return c.Store.Transact(ctx, func(tx DomainTx) error { return tx.UpdateCommand(command) })
}

func (c CommandService) SetStatus(ctx context.Context, command CommandRecord, status string, result []byte) error {
	if c.Store == nil || c.Project == "" {
		return errors.New("command service store and project identity are required")
	}
	command.ProjectIdentity = c.Project
	command.Status = status
	command.Result = append([]byte(nil), result...)
	return c.Store.Transact(ctx, func(tx DomainTx) error { return tx.UpdateCommand(command) })
}
