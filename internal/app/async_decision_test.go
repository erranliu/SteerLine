package app

import (
	api "github.com/erranliu/SteerLine/internal/product/api"
	"testing"
)

func TestAsyncDecisionClientPreservesInteraction(t *testing.T) {
	page := &api.CandidateSummary{Decision: &api.DecisionSummary{Status: "pending", Request: api.DecisionRequest{InteractionKind: "async", BlocksProgress: false, HasNativeQuestions: true}}}
	native := runtimeDecisionFacts(page)
	if native.Decision.Request.InteractionKind != "async" || native.Decision.Request.BlocksProgress || native.Decision.Request.QuestionRoute == nil {
		t.Fatal("client dropped async eligibility facts")
	}
}
