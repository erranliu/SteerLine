package steerline

import (
	"context"
	"errors"
	"strings"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

// Host-evidence follow-up keeps its original platform question route until #65
// moves these host actions behind the shared environment evidence port.
func nativeOrcaAnswerText(answer DecisionAnswer, request DecisionRequest) string {
	if request.QuestionRoute == nil {
		return workerFeedbackText(answer, request)
	}
	return strings.Join(answer.Answers[request.QuestionRoute.QuestionID], "\n\n")
}

func decodeNativeOrcaQuestionRoute(run *Run, execution *ExecutionRecord, route *executionapi.QuestionRoute) (*implementationTurnView, orcaworker.QuestionRouteData, error) {
	state, err := decodeOrcaExecutionState(execution)
	if err != nil {
		return nil, orcaworker.QuestionRouteData{}, err
	}
	if run == nil || execution == nil || execution.Process == nil || state == nil || route == nil {
		return nil, orcaworker.QuestionRouteData{}, errors.New("question route has no fixed execution or recipient")
	}
	for i := range state.Turns {
		turn := &state.Turns[i]
		fixed := workerSessionContext(run, execution)
		fixed.TaskID, fixed.CurrentTurnID = turn.TaskID, turn.TurnID
		fixed.Generation = nativeOrcaGeneration(execution)
		data, err := orcaworker.DecodeQuestionRoute(*route, *fixed, turn.TaskID, turn.DispatchID, execution.Process.Terminal)
		if err == nil {
			return turn, data, nil
		}
	}
	return nil, orcaworker.QuestionRouteData{}, errors.New("question route does not match a saved Turn, Dispatch and recipient")
}

func findNativeOrcaReply(ctx context.Context, config Config, runID, dispatchID, questionID, answer string) (string, error) {
	return orcaworker.FindReply(ctx, nativeOrcaClient(config).Call, runID, dispatchID, questionID, answer)
}

func findNativeOrcaSend(ctx context.Context, config Config, runID, dispatchID, subject, body string) (string, error) {
	return orcaworker.FindSend(ctx, nativeOrcaClient(config).Call, runID, dispatchID, subject, body)
}
