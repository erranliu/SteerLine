package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/product/query"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

func TestActivityAbandonmentCommitsOwnerAndReceiptTogether(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "domain.db"))
	require(t, err)
	t.Cleanup(func() {
		if s != nil {
			_ = s.Close()
		}
	})
	const project = "p"
	id := strings.Repeat("a", 32)
	at := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	record := roadmap.Record{ID: id, Project: project, RequestID: id, Revision: 1, State: "accepted", Submission: "started", CreatedAt: at}
	require(t, s.SaveRoadmap(ctx, record, 0))
	command := ports.CommandRecord{ProjectIdentity: project, CommandID: "abandon", IdempotencyKey: "abandon", Kind: "activity_abandon", TargetID: id, RequestedBy: "test", Payload: []byte(`{}`), Status: "applied", CreatedAt: at.Add(time.Hour)}
	stop := record
	stop.Revision, stop.Cancelled, stop.State, stop.StopID = 2, true, "cancelled", command.CommandID
	// A failed receipt insert must roll back the original stop intent, too.
	other := command
	other.Kind = "other"
	_, _, err = s.EnqueueCommand(ctx, other)
	require(t, err)
	writeStop := func(ctx context.Context) error { return s.SaveRoadmap(ctx, stop, 1) }
	if err = s.CommitActivityAbandonment(ctx, "roadmap", id, command, writeStop); err == nil {
		t.Fatal("receipt conflict did not roll back abandonment")
	}
	stored, err := s.Roadmap(ctx, project, id)
	require(t, err)
	if stored.Cancelled || stored.Revision != 1 {
		t.Fatalf("stop was written despite receipt conflict: %+v", stored)
	}
	marks, err := s.ReadAbandonedActivities(ctx, project, "roadmap", []string{id})
	require(t, err)
	if len(marks) != 0 {
		t.Fatal("failed write left an archive marker")
	}
	command.CommandID, command.IdempotencyKey, stop.StopID = "original", "original", "original"
	require(t, s.CommitActivityAbandonment(ctx, "roadmap", id, command, writeStop))
	stored, err = s.Roadmap(ctx, project, id)
	require(t, err)
	if !stored.Cancelled || stored.StopID != command.CommandID || stored.Settled() {
		t.Fatalf("original unsettled stop intent was not preserved: %+v", stored)
	}
	receipt, found, err := s.LookupCommand(ctx, project, command.CommandID)
	require(t, err)
	if !found || receipt.Status != "applied" {
		t.Fatalf("original receipt missing: %+v", receipt)
	}
	for _, filter := range []string{"ongoing", "attention", "recent", "history"} {
		page, err := query.ReadAsyncActivities(ctx, s, project, "i", query.AsyncActivityOptions{Filter: filter, Sources: []string{"roadmap"}, RequestIDs: []string{id}})
		require(t, err)
		want := 0
		if filter == "recent" || filter == "history" {
			want = 1
		}
		linked := page.Links[id]
		if len(page.Items) != want || page.Counts[filter] != want || linked.AbandonedAt == nil || !linked.AbandonedAt.Equal(command.CreatedAt) || linked.State != "waiting" {
			t.Fatalf("%s: archive membership and original unsettled state diverged: %+v", filter, page)
		}
	}
	second := command
	second.CommandID, second.IdempotencyKey, second.CreatedAt = "second", "second", command.CreatedAt.Add(time.Hour)
	require(t, s.CommitActivityAbandonment(ctx, "roadmap", id, second, nil))
	marks, err = s.ReadAbandonedActivities(ctx, project, "roadmap", []string{id})
	require(t, err)
	if marks[id] != command.CreatedAt.Format(time.RFC3339Nano) {
		t.Fatalf("duplicate changed the archive timestamp: %v", marks)
	}
	if _, err = s.RoadmapRequest(ctx, project, stop.StopID); errors.Is(err, roadmap.ErrNotFound) {
		t.Fatal("original stop request is no longer linked")
	}
	require(t, err)
	path := s.path
	require(t, s.Close())
	s = nil
	reopened, err := OpenSQLiteStore(path)
	require(t, err)
	defer reopened.Close()
	marks, err = reopened.ReadAbandonedActivities(ctx, project, "roadmap", []string{id})
	require(t, err)
	if marks[id] != command.CreatedAt.Format(time.RFC3339Nano) {
		t.Fatalf("reopening lost the original archive timestamp: %v", marks)
	}
}
