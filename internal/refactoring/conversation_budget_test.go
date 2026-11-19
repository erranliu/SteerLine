package refactoring

import (
	"context"
	"encoding/json"
	m "github.com/erranliu/SteerLine/internal/refactoring/model"
	rc "github.com/erranliu/SteerLine/internal/runnercontract"
	decision "github.com/erranliu/SteerLine/internal/steeragent/maintenance"
	"strings"
	"testing"
	"time"
)

func auditRecord() m.Record {
	head, clean := strings.Repeat("a", 40), true
	return m.Record{Authorized: true, Phase: "audit", TargetID: "goal", TargetStart: strings.Repeat("b", 40), LatestOwner: "write-run",
		Frozen: m.Frozen{Repository: rc.Repository{BaseRevision: strings.Repeat("b", 40)}},
		Operations: []m.Operation{
			{ID: "write", TargetID: "goal", Purpose: "implement", State: "complete", Feedback: "已完成、验证并提交。",
				WorkspaceGit: &rc.WorkspaceGit{Verified: true, RunID: "write-run", Commit: &head, Clean: &clean},
				Run:          &rc.Run{RunID: "write-run", Status: "completed", StopStatus: "stopped", Execution: rc.Execution{AttemptID: "attempt", SessionID: "session", TurnID: "turn"}, Final: &rc.Final{Verified: true, NativeOutcome: "succeeded", CommandID: "command", AttemptID: "attempt", SessionID: "session", TurnID: "turn"}}},
			{ID: "audit", TargetID: "goal", Purpose: "audit", State: "complete"},
		}}
}

func TestNaturalReplyCannotReplaceTrustedExecutionFacts(t *testing.T) {
	for _, change := range []string{"valid", "same-head", "dirty", "missing-git", "failed", "foreign-final", "foreign-target"} {
		t.Run(change, func(t *testing.T) {
			r := auditRecord()
			o := &r.Operations[0]
			switch change {
			case "same-head":
				*o.WorkspaceGit.Commit = r.TargetStart
			case "dirty":
				*o.WorkspaceGit.Clean = false
			case "missing-git":
				o.WorkspaceGit = nil
			case "failed":
				o.Run.Status = "failed"
				o.Run.Final.NativeOutcome = "failed"
			case "foreign-final":
				o.Run.Final.TurnID = "other"
			case "foreign-target":
				o.TargetID = "other"
			}
			s := &Service{}
			if err := s.applyDecision(&r, "audit", decision.Decision{Kind: "accept", Summary: "Runner 报告已验证"}); err != nil {
				t.Fatal(err)
			}
			accepted := r.Operations[1].AcceptedOperation == "write"
			if accepted != (change == "valid") {
				t.Fatalf("%s accepted=%v phase=%s", change, accepted, r.Phase)
			}
			if change == "valid" && (!r.FinishRequested || r.AcceptedHead() == "" || r.CloseReason == "") {
				t.Fatal("default target did not close with accepted work")
			}
		})
	}
}

func TestAuditFinishRequiresCleanUnchangedWorkspace(t *testing.T) {
	for _, change := range []string{"unchanged", "dirty", "changed", "missing"} {
		t.Run(change, func(t *testing.T) {
			r := auditRecord()
			*r.Operations[0].WorkspaceGit.Commit = r.TargetStart
			switch change {
			case "dirty":
				*r.Operations[0].WorkspaceGit.Clean = false
			case "changed":
				*r.Operations[0].WorkspaceGit.Commit = strings.Repeat("a", 40)
			case "missing":
				r.Operations[0].WorkspaceGit = nil
			}
			s := &Service{}
			_ = s.applyDecision(&r, "audit", decision.Decision{Kind: "finish", Summary: "此目标无需改动"})
			if (r.Phase == "verifying") != (change == "unchanged") {
				t.Fatal("finish lost work", change, r.Phase)
			}
		})
	}
}

func TestBudgetChangesQueuedAuditInstructionBeforeDispatch(t *testing.T) {
	s, f, r := fixture(t)
	now := s.now()
	s.Now = func() time.Time { return now }
	var err error
	r, err = s.update(context.Background(), r.ID, func(r *m.Record) error { r.Input.Budget = m.Budget{Minutes: "10"}; return nil })
	if err != nil {
		t.Fatal(err)
	}
	r = tickUntil(t, s, r.ID, func(r m.Record) bool {
		return r.Phase == "audit" && r.Operations[len(r.Operations)-1].Purpose == "audit" && r.Operations[len(r.Operations)-1].State == "pending"
	})
	old := r.Operations[len(r.Operations)-1]
	before := len(f.requests)
	now = now.Add(11 * time.Minute)
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Operations[len(r.Operations)-1].State == "cancelled" })
	if len(f.requests) != before || !r.FinishRequested {
		t.Fatal("stale permission was sent")
	}
	r = tickUntil(t, s, r.ID, func(r m.Record) bool {
		return r.Operations[len(r.Operations)-1].Purpose == "audit" && r.Operations[len(r.Operations)-1].State == "pending"
	})
	latest := r.Operations[len(r.Operations)-1]
	var in rc.GenerateRunInput
	if err = json.Unmarshal(latest.Request.Input, &in); err != nil {
		t.Fatal(err)
	}
	if latest.Request.RequestID == old.Request.RequestID || !latest.Closing || !strings.Contains(in.Generate.Instruction, "当前应收尾交付") {
		t.Fatal("audit permission was not refreshed")
	}
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
	if r.PR == nil {
		t.Fatal("closing lost the accepted work")
	}
}

func TestBudgetClosingCancelsUnsentImplementation(t *testing.T) {
	s, f, r := fixture(t)
	now := s.now()
	s.Now = func() time.Time { return now }
	_, err := s.update(context.Background(), r.ID, func(r *m.Record) error { r.Input.Budget = m.Budget{Minutes: "10"}; return nil })
	if err != nil {
		t.Fatal(err)
	}
	r = tickUntil(t, s, r.ID, func(r m.Record) bool {
		return r.Phase == "execute" && r.Operations[len(r.Operations)-1].Purpose == "implement"
	})
	before := len(f.requests)
	now = now.Add(11 * time.Minute)
	r = tickUntil(t, s, r.ID, func(r m.Record) bool { return r.Phase == "delivered" })
	if r.PR != nil || len(f.requests) != before || r.CloseReason == "" {
		t.Fatal("expired budget sent a new target")
	}
}

func TestContextPairsMultipleAnswersWithTheirQuestions(t *testing.T) {
	first := &m.Question{ID: "first", QuestionBody: m.QuestionBody{Prompt: "first question"}}
	second := &m.Question{ID: "second", QuestionBody: m.QuestionBody{Prompt: "second question"}}
	r := auditRecord()
	r.Answers = []m.Answer{{QuestionID: first.ID, Question: first}, {QuestionID: second.ID, Question: second}}
	r.Operations = append(r.Operations, m.Operation{Purpose: "audit", Question: first}, m.Operation{Purpose: "audit", Question: second})
	s := &Service{}
	c := s.context(r, m.ReviewEvidence{})
	if c.Question != second || len(c.Answers) != 2 || c.Answers[0].Question.Prompt != "first question" || c.Answers[1].Question.Prompt != "second question" {
		t.Fatal("answer context lost question identity", c)
	}
}

func TestBudgetAllowsNextTargetWithPreviousAcceptedBaseline(t *testing.T) {
	r := auditRecord()
	r.Input.Budget = m.Budget{Minutes: "100"}
	s := &Service{}
	if err := s.applyDecision(&r, "audit", decision.Decision{Kind: "accept", Summary: "first accepted"}); err != nil {
		t.Fatal(err)
	}
	firstHead := r.AcceptedHead()
	if r.Phase != "select" || r.FinishRequested || firstHead == "" {
		t.Fatal("remaining budget did not allow selection")
	}
	r.Operations = append(r.Operations, m.Operation{ID: "next-target", Purpose: "select", State: "complete"})
	if err := s.applyDecision(&r, "select", decision.Decision{Kind: "execute", Prompt: "next improvement", Summary: "selected"}); err != nil {
		t.Fatal(err)
	}
	if r.TargetID != "next-target" || r.TargetStart != firstHead {
		t.Fatal("new target lost previous accepted baseline")
	}
	second := auditRecord().Operations[0]
	second.ID, second.TargetID = "second-write", r.TargetID
	*second.WorkspaceGit.Commit = strings.Repeat("c", 40)
	r.Operations = append(r.Operations, second, m.Operation{ID: "second-audit", TargetID: r.TargetID, Purpose: "audit", State: "complete"})
	if err := s.applyDecision(&r, "audit", decision.Decision{Kind: "accept", Summary: "second accepted"}); err != nil {
		t.Fatal(err)
	}
	if r.AcceptedHead() != strings.Repeat("c", 40) || r.Operations[len(r.Operations)-1].AcceptedOperation != second.ID {
		t.Fatal("second audit accepted the wrong operation")
	}
}
