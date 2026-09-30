package steerline

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type countingEventStore struct {
	DomainStore
	calls               int
	maxPayloadBytesRead int
}

func (s *countingEventStore) Events(ctx context.Context, project, execution, attempt string, after int64, limit, payloadBudget int) ([]DomainEvent, error) {
	s.calls++
	events, err := s.DomainStore.Events(ctx, project, execution, attempt, after, limit, payloadBudget)
	bytes := 0
	for _, event := range events {
		bytes += len(event.Payload)
	}
	s.maxPayloadBytesRead = max(s.maxPayloadBytesRead, bytes)
	return events, err
}

func TestQueryEventBatchPagingPreservesByteBoundary(t *testing.T) {
	ctx := context.Background()
	store := openDomainTestStore(t)
	project := "event-batch"
	bodies := []string{strings.Repeat("a", 700<<10), strings.Repeat("b", 500<<10), strings.Repeat("c", MaxEventPageBytes+1), "", "tail"}
	ids := []int64{}
	require(t, store.Transact(ctx, func(tx DomainTx) error {
		for i, body := range bodies {
			id, err := tx.AppendEvent(DomainEvent{ProjectIdentity: project, ExecutionID: "execution", AttemptID: "attempt", ExternalSource: "batch", ExternalID: fmt.Sprint(i), Type: "message", Payload: []byte(body)})
			if err != nil {
				return err
			}
			ids = append(ids, id)
		}
		for i, scope := range []EventFilter{{ExecutionID: "other", AttemptID: "attempt"}, {ExecutionID: "execution", AttemptID: "other"}} {
			if _, err := tx.AppendEvent(DomainEvent{ProjectIdentity: project, ExecutionID: scope.ExecutionID, AttemptID: scope.AttemptID, ExternalSource: "batch", ExternalID: fmt.Sprint(i + 10), Type: "message", Payload: []byte("wrong scope")}); err != nil {
				return err
			}
		}
		_, err := tx.AppendEvent(DomainEvent{ProjectIdentity: "other-project", ExecutionID: "execution", AttemptID: "attempt", ExternalSource: "batch", ExternalID: "other", Type: "message", Payload: []byte("wrong project")})
		return err
	}))
	counted := &countingEventStore{DomainStore: store}
	query := NewQueryService(counted, project)
	filter := EventFilter{ExecutionID: "execution", AttemptID: "attempt"}
	first, err := query.ReadEvents(ctx, filter, "", 200)
	require(t, err)
	if len(first.Events) != 1 || first.Events[0].EventID != ids[0] || first.PayloadBytes != len(bodies[0]) || !first.HasMore || first.NextCursor == "" || counted.calls != 1 {
		t.Fatalf("batch read lost the byte boundary: events=%d bytes=%d calls=%d", len(first.Events), first.PayloadBytes, counted.calls)
	}
	second, err := query.ReadEvents(ctx, filter, first.NextCursor, 200)
	require(t, err)
	if len(second.Events) != 4 || second.HasMore || second.NextCursor != "" || second.PayloadBytes != len(bodies[1])+len(bodies[4]) || counted.calls != 2 {
		t.Fatalf("continuation skipped prefetched events or mixed scopes: %+v calls=%d", second, counted.calls)
	}
	for i, event := range second.Events {
		if event.EventID != ids[i+1] {
			t.Fatal("event cursor skipped or repeated an event")
		}
	}
	large := second.Events[1]
	if !large.PayloadOmitted || len(large.Payload) != 0 || large.PayloadDigest != operationDigest([]byte(bodies[2])) {
		t.Fatal("oversized body or its reference was lost during batch paging")
	}
	body, err := query.ReadEventBody(ctx, large.EventID, 0, 100)
	require(t, err)
	if body.TotalBytes != int64(len(bodies[2])) || string(body.Data) != bodies[2][:100] {
		t.Fatal("batch paging changed separate body reads")
	}
}

func TestQueryEventBatchReadCountAndPageLimit(t *testing.T) {
	ctx := context.Background()
	store := openDomainTestStore(t)
	const project = "event-count"
	require(t, store.Transact(ctx, func(tx DomainTx) error {
		for i := 0; i < MaxEventPageSize+1; i++ {
			if _, err := tx.AppendEvent(DomainEvent{ProjectIdentity: project, ExecutionID: "execution", ExternalSource: "count", ExternalID: fmt.Sprint(i), Type: "message", Payload: []byte("small")}); err != nil {
				return err
			}
		}
		return nil
	}))
	for _, limit := range []int{0, 1, eventReadBatchSize, eventReadBatchSize + 1, MaxEventPageSize + 1} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			counted := &countingEventStore{DomainStore: store}
			query := NewQueryService(counted, project)
			page, err := query.ReadEvents(ctx, EventFilter{ExecutionID: "execution"}, "", limit)
			require(t, err)
			expected := limit
			if expected == 0 {
				expected = DefaultEventPageSize
			}
			expected = min(expected, MaxEventPageSize)
			calls := (expected+eventReadBatchSize-1)/eventReadBatchSize + 1
			if len(page.Events) != expected || !page.HasMore || counted.calls != calls {
				t.Fatalf("page bounds or batch query count changed: events=%d calls=%d want=%d/%d", len(page.Events), counted.calls, expected, calls)
			}
			last, err := query.ReadEvents(ctx, EventFilter{ExecutionID: "execution"}, page.NextCursor, MaxEventPageSize)
			require(t, err)
			if last.Events[0].EventID <= page.Events[len(page.Events)-1].EventID || last.HasMore || len(last.Events)+expected != MaxEventPageSize+1 {
				t.Fatal("count-limited page did not continue exactly once to the end")
			}
		})
	}
}

func TestQueryEventBatchDoesNotPrefetchUnusedLargeBodies(t *testing.T) {
	ctx := context.Background()
	store := openDomainTestStore(t)
	const project = "event-payload-budget"
	body := []byte(strings.Repeat("a", MaxEventPageBytes))
	ids := []int64{}
	require(t, store.Transact(ctx, func(tx DomainTx) error {
		for i := 0; i < 4; i++ {
			id, err := tx.AppendEvent(DomainEvent{ProjectIdentity: project, ExecutionID: "execution", ExternalSource: "budget", ExternalID: fmt.Sprint(i), Type: "message", Payload: body})
			if err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return nil
	}))
	counted := &countingEventStore{DomainStore: store}
	query := NewQueryService(counted, project)
	cursor := ""
	for i, id := range ids {
		page, err := query.ReadEvents(ctx, EventFilter{ExecutionID: "execution"}, cursor, 200)
		require(t, err)
		if len(page.Events) != 1 || page.Events[0].EventID != id || page.Events[0].PayloadOmitted || len(page.Events[0].Payload) != MaxEventPageBytes || page.HasMore != (i < len(ids)-1) {
			t.Fatal("byte-bounded batches skipped or omitted a usable event body")
		}
		if counted.maxPayloadBytesRead > MaxEventPageBytes {
			t.Fatalf("batch allocated %d bytes for a %d-byte page", counted.maxPayloadBytesRead, MaxEventPageBytes)
		}
		cursor = page.NextCursor
	}
}
