package execution

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	contracts "github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/query"
)

func TestActivityDatabaseSurvivesReopenWithoutMirror(t *testing.T) {
	s, run := neutralAuditService(t)
	ctx := context.Background()
	dir := t.TempDir()
	log := filepath.Join(dir, "events.jsonl")
	raw := []byte("original raw evidence\r\n")
	require(t, os.WriteFile(log, raw, 0600))
	// A directory at the obsolete mirror path must not affect event commitment.
	require(t, os.Mkdir(filepath.Join(dir, "activity.jsonl"), 0700))
	run.LogPath = &log
	event := contracts.ActivityEvent{Source: "fixture-stream", Kind: "message", ItemID: "original-item", Detail: "original activity", TS: now()}
	s.recordActivity(run, event)
	require(t, s.commitEvents())
	if s.State.Service.Error != nil {
		t.Fatal("obsolete mirror path affected the activity transaction")
	}
	config, runID := s.Config, run.RunID
	require(t, s.Close())
	reopened, err := newNeutralService(config)
	require(t, err)
	defer reopened.Close()
	run = reopened.find(runID)
	reopened.recordActivity(run, event)
	require(t, reopened.commitEvents())
	page, err := query.NewService(testSQLite(reopened), config.Identity()).ReadEvents(ctx, query.EventFilter{ExecutionID: run.ExecutionID, AttemptID: run.AttemptID}, "", 100)
	require(t, err)
	count := 0
	for _, stored := range page.Events {
		if stored.Type != "activity.message" {
			continue
		}
		count++
		var got contracts.ActivityEvent
		require(t, json.Unmarshal(stored.Payload, &got))
		if stored.EventID <= 0 || stored.WorkflowRunID != runID || got.RunID != runID || got.Detail != event.Detail || got.V != ActivityVersion {
			t.Fatal("database activity lost its original scope or payload")
		}
	}
	if count != 1 || reopened.State.Service.Error != nil {
		t.Fatal("mirror interference or reopen duplicated the canonical activity")
	}
	got, err := os.ReadFile(log)
	require(t, err)
	if string(got) != string(raw) {
		t.Fatal("activity recording changed raw evidence")
	}
}
