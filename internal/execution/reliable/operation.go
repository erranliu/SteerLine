package reliable

import (
	"encoding/json"
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func InputValue(action executionbackend.LifecycleAction, input executionbackend.LifecycleInput) (any, error) {
	if err := input.Validate(action); err != nil {
		return nil, err
	}
	switch action {
	case executionbackend.LifecyclePrepareWorkspace:
		return input.Workspace, nil
	case executionbackend.LifecycleCreateResource:
		return input.Resource, nil
	case executionbackend.LifecyclePrepareSession, executionbackend.LifecycleStartTurn, executionbackend.LifecycleSubmitInput:
		return input.Turn, nil
	case executionbackend.LifecycleReportCompletion:
		return input.Completion, nil
	case executionbackend.LifecycleDeliverFeedback:
		return input.Feedback, nil
	case executionbackend.LifecycleAcknowledge:
		return input.Ack, nil
	case executionbackend.LifecycleStopObservation, executionbackend.LifecycleStopExecution:
		return input.Stop, nil
	case executionbackend.LifecycleCleanup:
		return input.Cleanup, nil
	case executionbackend.LifecycleHostAction:
		return input.Host, nil
	default:
		return nil, errors.New("unsupported lifecycle operation action")
	}
}

func OperationKey(executionID string, action executionbackend.LifecycleAction, input executionbackend.LifecycleInput) (string, any, error) {
	value, err := InputValue(action, input)
	if err != nil {
		return "", nil, err
	}
	wire, err := json.Marshal(value)
	if err != nil {
		return "", nil, err
	}
	return "worker-lifecycle:" + executionID + ":" + string(action) + ":" + Digest(wire), value, nil
}
