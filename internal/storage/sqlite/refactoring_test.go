package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/product/query"
	m "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/storage/ports"
	"testing"
	"time"
)

func TestRefactoringTransactionalSlotCASAndActivity(t *testing.T) {
	s := openDomainTestStore(t)
	ctx := context.Background()
	r := m.Record{ID: m.ID(), Project: "p", Revision: 1, Phase: "select", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	cmd := m.Command{ID: r.ID, Raw: json.RawMessage(`{"start":1}`), Action: "refactoring_start", Target: "p", Expected: 1}
	if e := s.SaveRefactoring(ctx, r, 0, &cmd); e != nil {
		t.Fatal(e)
	}
	other := r
	other.ID = m.ID()
	if e := s.SaveRefactoring(ctx, other, 0, nil); !errors.Is(e, m.ErrConflict) {
		t.Fatal("parallel action admitted", e)
	}
	r.Revision++
	r.Phase = "stopped"
	if e := s.SaveRefactoring(ctx, r, 1, nil); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveRefactoring(ctx, r, 1, nil); !errors.Is(e, m.ErrConflict) {
		t.Fatal("stale CAS accepted", e)
	}
	if e := s.SaveRefactoring(ctx, other, 0, nil); e != nil {
		t.Fatal(e)
	}
	r.Revision++
	r.Phase = "finish"
	if e := s.SaveRefactoring(ctx, r, 2, nil); !errors.Is(e, m.ErrConflict) {
		t.Fatal("finish ignored current slot", e)
	}
	raw, ack, e := s.RefactoringCommand(ctx, "p", cmd.ID)
	if e != nil || string(raw) != string(cmd.Raw) || len(ack) > 200 {
		t.Fatal("immutable minimal receipt", string(raw), string(ack), e)
	}
	facts, _, e := s.ReadAsyncActivityFacts(ctx, "p", ports.AsyncActivityRead{Source: "refactoring", Filter: "summary", Limit: 50})
	if e != nil || len(facts) != 2 {
		t.Fatal(facts, e)
	}
	page, e := query.ReadAsyncActivities(ctx, s, "p", "instance", query.AsyncActivityOptions{Sources: []string{"refactoring"}, Filter: "ongoing"})
	if e != nil || len(page.Items) != 1 || page.Items[0].SteerAgent.Type != "maintenance" {
		t.Fatal(page, e)
	}
}
