package query

import (
	m "github.com/erranliu/SteerLine/internal/execution/model"
	"testing"
)

func TestAsyncQuestionPageDerivesBlockingWithoutChangingRequest(t *testing.T) {
	request := m.DecisionRequest{InteractionKind: "async", Question: "Publisher?"}
	run := &CandidateView{BusinessStatus: "running", Decision: &m.DecisionState{Request: request, Status: "pending"}}
	if Candidate(run).Decision.Request.BlocksProgress {
		t.Fatal("running async question blocks progress")
	}
	run.BusinessStatus = "waiting_input"
	if !Candidate(run).Decision.Request.BlocksProgress || run.Decision.Request.BlocksProgress {
		t.Fatal("page did not derive blocking or mutated immutable question")
	}
}
