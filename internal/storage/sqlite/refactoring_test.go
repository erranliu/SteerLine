package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/product/query"
	m "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/ports"
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

func TestRefactoringReadingUsesOriginalPublicFields(t *testing.T) {
	s := openDomainTestStore(t)
	ctx := context.Background()
	final := &runnercontract.Final{Verified: true, AttemptID: "attempt-1", SessionID: "session", TurnID: "turn", Text: strings.Repeat("公开回复", 100)}
	r := m.Record{ID: m.ID(), Project: "p", Revision: 1, Phase: "stopped",
		Operations: []m.Operation{
			{ID: "operation", Purpose: "execute", State: "complete", Prompt: "内部指令",
				RunID: "run", Run: &runnercontract.Run{RunID: "run", Final: final},
				Result: &m.Result{Summary: "操作结果"}},
		},
		Answers: []m.Answer{{QuestionID: "q", Text: "用户回答"}},
	}
	if err := s.SaveRefactoring(ctx, r, 0, nil); err != nil {
		t.Fatal(err)
	}
	if version, err := s.ReadingVersion(ctx, "p", "refactoring", r.ID); err != nil || version != "1" {
		t.Fatalf("missing reading version: %s, %v", version, err)
	}
	read := func(attempt string) {
		t.Helper()
		page, err := s.ReadReadingWindow(ctx, "p", "refactoring", r.ID, 0, 0, 10)
		if err != nil || len(page.Records) != 3 {
			t.Fatalf("expected only the reply, result, and answer: %+v, %v", page, err)
		}
		item := page.Records[0]
		if item.Role != "assistant" || item.RunID != "run" || item.AttemptID != attempt || item.Group != "操作 1" || len(item.Preview) >= item.Bytes ||
			page.Records[1].Kind != "generated_result" || page.Records[2].Role != "user" || page.Records[2].AttemptID != "" {
			t.Fatalf("incorrect role, provenance, or bounds: %+v", page)
		}
		body, err := s.ReadReadingBody(ctx, "p", "refactoring", r.ID, item.ID, item.Bytes)
		if err != nil || body != final.Text {
			t.Fatalf("did not read the original reply: %v", err)
		}
	}
	read("attempt-1")

	r.Revision++
	final.AttemptID = "attempt-2"
	if err := s.SaveRefactoring(ctx, r, 1, nil); err != nil {
		t.Fatal(err)
	}
	read("attempt-2")
}
