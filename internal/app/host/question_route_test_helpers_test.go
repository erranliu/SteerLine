package host

import (
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func simpleQuestionRoute(t *testing.T, implementation, project, workflow, execution, attempt, generation, turn, question string) executionbackend.ImplementationDataEnvelope {
	t.Helper()
	route, err := executionbackend.NewQuestionRoute(implementation, 1, executionbackend.QuestionRouteIdentity{
		ProjectID: project, WorkflowRunID: workflow, ExecutionID: execution, AttemptID: attempt,
		Generation: generation, TurnID: turn, QuestionID: question,
	}, []byte(`{"route":"fixed"}`))
	if err != nil {
		t.Fatal(err)
	}
	return route
}
