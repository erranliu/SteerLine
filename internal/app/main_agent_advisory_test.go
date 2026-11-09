package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
)

func TestMainAgentAdvisorySourceIsExplicitAndFreezesOwnerFacts(t *testing.T) {
	c := runnerstate.Candidate{ID: strings.Repeat("a", 32), TaskID: "1", Participation: "active", BaseRevision: "fixed", ContractDigest: "contract", Projection: &runnercontract.Run{Revision: 7, Execution: runnercontract.Execution{AttemptID: "attempt"}, Question: &runnercontract.Question{ID: "question", Text: "Conflicting acceptance requirements"}, Controls: runnercontract.Controls{Reason: "requirements_conflict"}}}
	s, ok := materialAdviceSource(c)
	if !ok || s.Occurrence != "attempt:question" || len(s.Facts) != 1 {
		t.Fatal(s, ok)
	}
	c.Projection.Question.Text = "later observation"
	if s.Goal != "Conflicting acceptance requirements" || !strings.Contains(s.Facts[0].Body, "Conflicting acceptance requirements") {
		t.Fatal("source was not frozen")
	}
	for _, reason := range []string{"waiting_input", "blocked", "needs_attention", "permission_required", ""} {
		c.Projection.Controls.Reason = reason
		if _, ok := materialAdviceSource(c); ok {
			t.Fatal("ordinary state was classified", reason)
		}
	}
}
func TestMainAgentAdvisoryLegacyActionsRejectedBeforeBusinessAccess(t *testing.T) {
	// No Server, actor, store or Runner: accidental access would panic.
	p := &runtimeProjectPort{}
	for _, action := range []string{"main_agent_answer", "main_agent_publish", "main_agent_prepare", "main_agent_analyze", "main_agent_suggest"} {
		_, err := p.SubmitMainAgent(context.Background(), httpcontract.CommandEnvelope{Action: action, Payload: json.RawMessage(`{}`)})
		if err == nil || !strings.Contains(err.Error(), "unsupported") {
			t.Fatal(action, err)
		}
	}
}
