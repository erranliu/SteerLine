package steerline

import (
	"testing"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)


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
