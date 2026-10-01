package steerline

import "github.com/erranliu/SteerLine/internal/steerline/testprotocol"

type fakeNativeOrcaState = testprotocol.NativeOrcaState

func fakeNativeOrca(stateDir string, args []string) int {
	return testprotocol.FakeNativeOrca(stateDir, args)
}
