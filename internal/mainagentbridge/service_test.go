package mainagentbridge

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/mainagent/contract"
)

type memory struct {
	mu   sync.Mutex
	rows map[string][]byte
}

func (m *memory) LoadAdvisory(_ context.Context, p, id string) ([]byte, uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	raw, ok := m.rows[p+id]
	if !ok {
		return nil, 0, contract.ErrNotFound
	}
	var r Record
	_ = json.Unmarshal(raw, &r)
	return append([]byte{}, raw...), r.Revision, nil
}
func (m *memory) SaveAdvisory(_ context.Context, p, id, candidate string, raw []byte, expected uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var old Record
	_ = json.Unmarshal(m.rows[p+id], &old)
	if old.Revision != expected {
		return contract.ErrConflict
	}
	m.rows[p+id] = append([]byte{}, raw...)
	return nil
}
func (m *memory) AdvisoryIDs(_ context.Context, p, scope string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := []string{}
	for _, raw := range m.rows {
		var r Record
		_ = json.Unmarshal(raw, &r)
		if r.Project == p && (scope == "active" && r.SourceActive || scope == "pending" && !r.Complete()) {
			ids = append(ids, r.ID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}
func (m *memory) AdvisorySummaries(context.Context, string, string) ([]json.RawMessage, bool, error) {
	return nil, false, nil
}
func fixture() (*Service, Source, *int) {
	calls := new(int)
	source := Source{CandidateID: strings.Repeat("b", 32), Occurrence: "original-attempt:question", Revision: 1, Reason: "pending_question", Goal: "Empty input is unspecified"}
	s := &Service{Project: "project", Store: &memory{rows: map[string][]byte{}}, Build: func(_ context.Context, source Source, id string) (contract.Request, error) {
		*calls++
		return contract.Request{Protocol: contract.Protocol, RequestID: id, Capability: contract.Capability, Context: contract.Context{Goal: source.Goal, Materials: []contract.Material{}}}, nil
	}}
	attachFake(s)
	return s, source, calls
}
func TestAdvisoryAutomaticOccurrenceDedupAndGeneration(t *testing.T) {
	s, source, calls := fixture()
	ctx := context.Background()
	if err := s.Sync(ctx, []Source{source}); err != nil {
		t.Fatal(err)
	}
	id := SourceID(s.Project, source)
	queued, err := s.Get(ctx, id)
	if err != nil || queued.State != "queued" {
		t.Fatal(queued, err)
	}
	if err = s.ProcessNext(ctx); err != nil {
		t.Fatal(err)
	}
	r, err := s.Get(ctx, id)
	if err != nil || r.State != "queued" || r.Complete() || r.Request == nil || r.RunnerRequest == nil {
		t.Fatal(r, err)
	}
	for i := 0; i < 3; i++ {
		source.Revision++
		source.Goal = "later observation must not replace original"
		if err = s.Sync(ctx, []Source{source}); err != nil {
			t.Fatal(err)
		}
		if err = s.ProcessNext(ctx); err != nil {
			t.Fatal(err)
		}
	}
	// A reconstructed host shares persisted identities; no UI or model submission is needed.
	restarted := &Service{Project: s.Project, Store: s.Store, Build: s.Build}
	restarted.Runner, restarted.RunnerBuild = s.Runner, s.RunnerBuild
	if err = restarted.ProcessNext(ctx); err != nil {
		t.Fatal(err)
	}
	r, _ = s.Get(ctx, id)
	if *calls != 1 || r.Request.Context.Goal != "Empty input is unspecified" {
		t.Fatal("same occurrence reopened or context replaced", *calls, r)
	}
	if err = s.Sync(ctx, nil); err != nil {
		t.Fatal(err)
	}
	r, _ = s.Get(ctx, id)
	if r.SourceActive || r.State != "ready" {
		t.Fatal("source resolution rewrote historical result", r)
	}
}
func TestAdvisoryCancelledAndInvalidSourcesNeverPrepare(t *testing.T) {
	s, source, calls := fixture()
	ctx := context.Background()
	bad := source
	bad.Reason = "ordinary_question"
	if err := s.Sync(ctx, []Source{bad}); err == nil {
		t.Fatal("generic question classified")
	}
	if err := s.Sync(ctx, []Source{source}); err != nil {
		t.Fatal(err)
	}
	id := SourceID(s.Project, source)
	r, _ := s.Get(ctx, id)
	if _, err := s.Cancel(ctx, id, r.Revision+1); !errors.Is(err, contract.ErrConflict) {
		t.Fatal(err)
	}
	if _, err := s.Cancel(ctx, id, r.Revision); err != nil {
		t.Fatal(err)
	}
	if err := s.Sync(ctx, []Source{source}); err != nil {
		t.Fatal(err)
	}
	if err := s.ProcessNext(ctx); err != nil {
		t.Fatal(err)
	}
	if *calls != 0 {
		t.Fatal("cancelled occurrence prepared again")
	}
}
func TestAdvisoryCancellationWinsDuringSlowPreparation(t *testing.T) {
	s, source, _ := fixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	original := s.Build
	s.Build = func(ctx context.Context, a Source, id string) (contract.Request, error) {
		close(entered)
		<-ctx.Done()
		return original(ctx, a, id)
	}
	if err := s.Sync(ctx, []Source{source}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.ProcessNext(ctx) }()
	<-entered
	wait, cancelWait := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancelWait()
	r, err := s.Get(wait, SourceID(s.Project, source))
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.Cancel(wait, r.ID, r.Revision)
	if err != nil || r.State != "cancelled" {
		t.Fatal("preparing work was not cancelled", r, err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	r, err = s.Get(context.Background(), r.ID)
	if err != nil || r.State != "cancelled" || r.Request != nil {
		t.Fatal("late preparation overwrote cancellation", r, err)
	}
}
