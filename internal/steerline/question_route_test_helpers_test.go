package steerline

import (
	"encoding/json"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func codexDecisionFixture(t *testing.T, fixed executionapi.SessionContext, callID string, questions []executionapi.Question) DecisionRequest {
	t.Helper()
	for i := range questions {
		questions[i].ExecutionID = fixed.ExecutionID
		questions[i].AttemptID = fixed.AttemptID
		questions[i].Generation = fixed.Generation
		questions[i].TurnID = fixed.CurrentTurnID
	}
	call := codexworker.AskCall{Route: codexworker.AskRouteData{RequestID: json.RawMessage(`0`), ThreadID: fixed.SessionID, TurnID: fixed.CurrentTurnID, CallID: callID}, Questions: questions}
	route, err := codexworker.NewQuestionRoute(call, fixed)
	if err != nil {
		t.Fatal(err)
	}
	return decisionRequestFromEventFact(executionapi.EventFact{Questions: call.Questions, QuestionRoute: &route})
}

func simpleQuestionRoute(t *testing.T, implementation, project, workflow, execution, attempt, generation, turn, question string) executionapi.QuestionRoute {
	t.Helper()
	route, err := executionapi.NewQuestionRoute(implementation, 1, executionapi.QuestionRouteIdentity{
		ProjectID: project, WorkflowRunID: workflow, ExecutionID: execution, AttemptID: attempt,
		Generation: generation, TurnID: turn, QuestionID: question,
	}, []byte(`{"route":"fixed"}`))
	if err != nil {
		t.Fatal(err)
	}
	return route
}
