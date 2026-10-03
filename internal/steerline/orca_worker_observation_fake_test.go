package steerline

import (
	"github.com/erranliu/SteerLine/internal/steerline/testprotocol"
)

type fakeNativeOrcaState = testprotocol.NativeOrcaState

func fakeNativeOrca(stateDir string, args []string) int {
	return testprotocol.FakeNativeOrca(stateDir, args)
}

func nativeOrcaGeneration(execution *ExecutionRecord) string {
	if execution == nil {
		return ""
	}
	if state, err := decodeOrcaExecutionState(execution); err == nil && state != nil && state.Coordinator.RuntimeID != "" {
		return state.Coordinator.RuntimeID
	}
	if execution.Process != nil && execution.Process.Incarnation != "" {
		return execution.Process.Incarnation
	}
	return ""
}
