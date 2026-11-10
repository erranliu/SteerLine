package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/mainagent/contract"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

func advisoryRaw(id, candidate, state string, rev uint64) []byte {
	b, _ := json.Marshal(map[string]any{"project": "project", "id": id, "revision": rev, "source": map[string]any{"candidate_id": candidate, "task_id": "1", "goal": "private goal", "facts": []any{map[string]any{"body": "private source body"}}}, "source_active": true, "state": state, "settled": true, "created_at": "2026-11-09T00:00:00Z", "observed_at": "2026-11-09T00:00:00Z", "request": map[string]any{"private": "private context"}})
	return b
}
func TestAdvisoryStorageCASIsolationAndBoundedObservation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	id, candidate := strings.Repeat("a", 32), strings.Repeat("b", 32)
	if err = s.SaveAdvisory(ctx, "project", id, candidate, advisoryRaw(id, candidate, "queued", 1), 0); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveAdvisory(ctx, "project", id, candidate, advisoryRaw(id, candidate, "queued", 1), 0); !errors.Is(err, contract.ErrConflict) {
		t.Fatal(err)
	}
	if _, _, err = s.LoadAdvisory(ctx, "other", id); !errors.Is(err, contract.ErrNotFound) {
		t.Fatal("cross-project read", err)
	}
	if err = s.SaveAdvisory(ctx, "project", id, candidate, advisoryRaw(id, candidate, "blocked", 2), 1); err != nil {
		t.Fatal(err)
	}
	before, rev, err := s.LoadAdvisory(ctx, "project", id)
	if err != nil || rev != 2 {
		t.Fatal(rev, err)
	}
	for i := 0; i < 3; i++ {
		facts, _, err := s.ReadAsyncActivityFacts(ctx, "project", ports.AsyncActivityRead{Source: "main_agent_advisory", Filter: "summary", Limit: 50})
		if err != nil || len(facts) != 1 {
			t.Fatal(facts, err)
		}
		if strings.Contains(string(facts[0].Payload), "private") {
			t.Fatal("input bodies leaked into activity observation")
		}
	}
	after, _, _ := s.LoadAdvisory(ctx, "project", id)
	if string(before) != string(after) {
		t.Fatal("observation changed durable work")
	}
	summaries, more, err := s.AdvisorySummaries(ctx, "project", candidate)
	if err != nil || more || len(summaries) != 1 || strings.Contains(string(summaries[0]), "private") {
		t.Fatal(summaries, more, err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if raw, _, err := s.LoadAdvisory(ctx, "project", id); err != nil || string(raw) != string(before) {
		t.Fatal("durable request lost", err)
	}
}
