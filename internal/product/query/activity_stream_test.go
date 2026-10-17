package query

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	p "github.com/erranliu/SteerLine/internal/storage/ports"
)

type activityStream struct {
	rows                  []p.ActivityRecord
	reads, closes         int
	nextError, closeError error
}

func (s *activityStream) Next() (*p.ActivityRecord, error) {
	s.reads++
	if s.nextError != nil {
		return nil, s.nextError
	}
	if s.reads > len(s.rows) {
		return nil, nil
	}
	return &s.rows[s.reads-1], nil
}
func (s *activityStream) Close() error { s.closes++; return s.closeError }

type activitySource struct {
	stream *activityStream
	after  int64
}

func (s *activitySource) OpenActivityView(_ context.Context, project, candidate, run, attempt string, after int64, limit int, recent bool) (p.ActivityBinding, p.ActivityRecords, error) {
	s.after = after
	return p.ActivityBinding{ProjectIdentity: project, ExecutionWorkflowRunID: candidate, ExecutionID: run, AttemptID: attempt, AttemptExecutionID: run}, s.stream, nil
}
func activityRow(seq int64, kind string) p.ActivityRecord {
	return p.ActivityRecord{EventID: seq, TaskID: "task", Event: runnercontract.Event{EventID: "original-event", RunID: "execution", AttemptID: "attempt", Type: kind, CreatedAt: time.Now().UTC(), Final: &runnercontract.Final{Text: "private final body"}}}
}

func TestActivityProjectionBudgetStopsStreamWithoutAdvancingOverflow(t *testing.T) {
	kind := strings.Repeat("x", MaxEventPageBytes/4)
	stream := &activityStream{rows: []p.ActivityRecord{activityRow(10, kind), activityRow(11, kind), activityRow(12, "must not read")}}
	source := &activitySource{stream: stream}
	service := Service{Project: "project", Activity: source}
	cursor := api.ActivityCursor{RunID: "candidate", ExecutionID: "execution", AttemptID: "attempt", Initialized: true, AfterEventID: 9}
	before, _ := json.Marshal(stream.rows)
	page, err := service.ReadActivity(context.Background(), "candidate", "execution", "attempt", cursor)
	if err != nil || len(page.Activity) != 1 || page.Cursor.AfterEventID != 10 || source.after != 9 || stream.reads != 2 || stream.closes != 1 {
		t.Fatalf("budget/stream lifecycle: reads=%d closes=%d %+v %v", stream.reads, stream.closes, page, err)
	}
	encoded, _ := json.Marshal(page.Activity[0])
	if len(encoded) > MaxEventPageBytes || len(encoded)*2 <= MaxEventPageBytes || strings.Contains(string(encoded), "private final body") {
		t.Fatal("wrong projected budget or raw final leak")
	}
	after, _ := json.Marshal(stream.rows)
	if string(before) != string(after) {
		t.Fatal("activity projection mutated source facts")
	}
	source.stream = &activityStream{rows: []p.ActivityRecord{activityRow(11, kind)}}
	page, err = service.ReadActivity(context.Background(), "candidate", "execution", "attempt", page.Cursor)
	if err != nil || len(page.Activity) != 1 || page.Cursor.AfterEventID != 11 || source.after != 10 || source.stream.closes != 1 {
		t.Fatal("overflow record lost on continuation", err)
	}
	source.stream = &activityStream{rows: []p.ActivityRecord{activityRow(12, strings.Repeat("x", MaxEventPageBytes))}}
	page, err = service.ReadActivity(context.Background(), "candidate", "execution", "attempt", page.Cursor)
	if err != nil || len(page.Activity) != 0 || page.Cursor.AfterEventID != 11 || source.stream.reads != 1 || source.stream.closes != 1 {
		t.Fatal("first oversized activity changed established cursor behavior", err)
	}
}

func TestActivityProjectionClosesOnReadIdentityAndCommitErrors(t *testing.T) {
	failure := errors.New("read or close failed")
	for _, tc := range []struct {
		name   string
		stream *activityStream
	}{
		{"read failure", &activityStream{nextError: failure}},
		{"cancelled", &activityStream{nextError: context.Canceled}},
		{"close failure", &activityStream{rows: []p.ActivityRecord{activityRow(1, "accepted")}, closeError: failure}},
		{"foreign run", &activityStream{rows: []p.ActivityRecord{{EventID: 1, Event: runnercontract.Event{RunID: "foreign", AttemptID: "attempt"}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := Service{Project: "project", Activity: &activitySource{stream: tc.stream}}
			page, err := service.ReadActivity(context.Background(), "candidate", "execution", "attempt", api.ActivityCursor{})
			if err == nil || tc.stream.closes != 1 || len(page.Activity) != 0 {
				t.Fatal("failed stream leaked data or ownership", page, err, tc.stream.closes)
			}
		})
	}
}
