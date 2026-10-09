package controlled

import (
	"fmt"
	"time"

	testprotocol "github.com/erranliu/SteerLine/internal/testkit/protocol"
)

// ControlledOrcaStartup configures faults in an external CLI fixture, never in
// the production service or its database. Each project owns its fixture state.
func ControlledOrcaStartup(path, mode string) error {
	state := testprotocol.NativeOrcaState{StartOutcome: mode}
	switch mode {
	case "slow":
		state.StartDelay = 3 * time.Second
	case "unobserved", "failed":
		state.StartOutcome = mode
	case "lost":
		state.LoseDispatchAt = 1
	default:
		return fmt.Errorf("unknown controlled Orca startup fault %q", mode)
	}
	return testprotocol.WriteNativeOrcaState(path, state)
}

func ControlledOrcaMain(path string, args []string) int {
	return testprotocol.FakeNativeOrca(path, args)
}

type ControlledOrcaStartupState struct {
	Dispatches        int
	Stopped, Released bool
}

func ReadControlledOrcaStartup(path string) (ControlledOrcaStartupState, error) {
	state, err := testprotocol.ReadNativeOrcaState(path)
	return ControlledOrcaStartupState{Dispatches: state.Dispatches, Stopped: state.ClosedAgent, Released: state.ReleasedAgent}, err
}
