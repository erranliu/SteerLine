package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/product/query"
	m "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

func TestRefactoringEnvironmentProofIsAtomicWithDispatch(t *testing.T) {
	s := openDomainTestStore(t)
	ctx := context.Background()
	record := readinessRecord("project")
	record.State = "ready"
	record.Rounds[0].Settled = true
	require(t, s.StartEnvironment(ctx, record, 0))
	key, err := env.Key(record.Execution, record.Prepared.Identity)
	require(t, err)
	current, err := s.CurrentEnvironment(ctx, record.Project, key)
	require(t, err)
	proof, err := env.Qualify(current, record, record.Prepared.Inputs)
	require(t, err)
	first := m.Record{ID: m.ID(), Project: record.Project, Revision: 1, Phase: "select"}
	second := first
	second.ID = m.ID()
	require(t, s.SaveRefactoring(ctx, first, 0, nil, proof))
	require(t, s.SaveRefactoring(ctx, second, 0, nil, proof))
	first.Revision++
	first.Slot, first.Authorized = true, true
	require(t, s.SaveRefactoring(ctx, first, 1, nil, proof))
	invalidateReadiness(t, s, record.Project, key, proof.Revision)
	second.Revision++
	second.Slot, second.Authorized = true, true
	if err := s.SaveRefactoring(ctx, second, 1, nil, proof); !errors.Is(err, m.ErrEnvironmentChanged) {
		t.Fatal("revocation before authorization did not block dispatch", err)
	}
	saved, err := s.Refactoring(ctx, record.Project, second.ID)
	require(t, err)
	if saved.Slot || saved.Authorized || saved.Revision != 1 {
		t.Fatal("revoked dispatch changed original action", saved)
	}
	first.Revision++
	require(t, s.SaveRefactoring(ctx, first, 2, nil))
}

func TestRefactoringTransactionalSlotCASAndActivity(t *testing.T) {
	s := openDomainTestStore(t)
	ctx := context.Background()
	r := m.Record{ID: m.ID(), Project: "p", Revision: 1, Phase: "select", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	cmd := m.Command{ID: r.ID, Raw: json.RawMessage(`{"start":1}`), Action: "refactoring_start", Target: "p", Expected: 1}
	if e := s.SaveRefactoring(ctx, r, 0, &cmd); e != nil {
		t.Fatal(e)
	}
	others := make([]m.Record, m.Concurrency+1)
	for i := range others {
		others[i] = r
		others[i].ID = m.ID()
		others[i].Slot = i < m.Concurrency
		if e := s.SaveRefactoring(ctx, others[i], 0, nil); e != nil {
			t.Fatal("could not save three occupied slots and one queued action", e)
		}
	}
	r.Slot = true
	r.Revision++
	if e := s.SaveRefactoring(ctx, r, 1, nil); !errors.Is(e, m.ErrConflict) {
		t.Fatal("fourth slot admitted", e)
	}
	foreign := r
	foreign.Project, foreign.Revision = "second-project", 1
	if e := s.SaveRefactoring(ctx, foreign, 0, nil); e != nil {
		t.Fatal("first project's capacity affected second project", e)
	}
	r.Slot = false
	r.Phase = "stopped"
	if e := s.SaveRefactoring(ctx, r, 1, nil); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveRefactoring(ctx, r, 1, nil); !errors.Is(e, m.ErrConflict) {
		t.Fatal("stale CAS accepted", e)
	}
	r.Revision++
	r.Phase = "finish"
	if e := s.SaveRefactoring(ctx, r, 2, nil); e != nil {
		t.Fatal("finish should queue while full", e)
	}
	raw, ack, e := s.RefactoringCommand(ctx, "p", cmd.ID)
	if e != nil || string(raw) != string(cmd.Raw) || len(ack) > 200 {
		t.Fatal("immutable minimal receipt", string(raw), string(ack), e)
	}
	facts, _, e := s.ReadAsyncActivityFacts(ctx, "p", ports.AsyncActivityRead{Source: "refactoring", Filter: "summary", Limit: 50})
	if e != nil || len(facts) != len(others)+1 {
		t.Fatal(facts, e)
	}
	page, e := query.ReadAsyncActivities(ctx, s, "p", "instance", query.AsyncActivityOptions{Sources: []string{"refactoring"}, Filter: "ongoing"})
	if e != nil || len(page.Items) != len(others)+1 || page.Items[0].SteerAgent.Type != "maintenance" {
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
		Answers: []m.Answer{{QuestionID: "q", Option: m.QuestionOption{ID: "keep", Label: "保留公共接口"}, Note: "用户补充"}},
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
		if err != nil || len(page.Records) != 4 {
			t.Fatalf("expected the reply, result, answer option, and note: %+v, %v", page, err)
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
		for i, want := range []string{r.Answers[0].Option.Label, r.Answers[0].Note} {
			item := page.Records[i+2]
			body, err := s.ReadReadingBody(ctx, "p", "refactoring", r.ID, item.ID, item.Bytes)
			if err != nil || item.Role != "user" || body != want {
				t.Fatalf("did not read the original answer field: item=%+v body=%q err=%v", item, body, err)
			}
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
