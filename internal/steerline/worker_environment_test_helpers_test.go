package steerline

import (
	"context"
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

type implementationExecutionView = orcaworker.NativeExecutionState
type implementationTurnView = orcaworker.NativeTurnState

func orcaStateIdentity(record *ExecutionRecord) orcaworker.ExecutionStateIdentity {
	process := executionapi.ProcessIdentity{}
	if record.Process != nil {
		process = *record.Process
	}
	return orcaworker.ExecutionStateIdentity{ProjectID: record.ProjectIdentity, ExecutionID: record.ExecutionID, AttemptID: record.CurrentAttemptID,
		Generation: record.Generation, ConfigDigest: record.ConfigurationDigest, Protocol: executionapi.ExecutionProtocol(record.Protocol), Process: process,
		CoordinatorTerminal: record.Target.CoordinatorTerminal}
}

func decodeOrcaExecutionState(record *ExecutionRecord) (*implementationExecutionView, error) {
	envelope, ok, err := implementationEnvelope(record, orcaworker.ExecutionStateCodecKey)
	if err != nil || !ok {
		return nil, err
	}
	state, err := orcaworker.DecodeExecutionStateEnvelope(envelope, orcaStateIdentity(record))
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func storeOrcaExecutionState(record *ExecutionRecord, state *implementationExecutionView) error {
	if state == nil {
		return errors.New("test native execution state is required")
	}
	envelope, err := orcaworker.NewExecutionStateEnvelope(orcaStateIdentity(record), *state)
	if err != nil {
		return err
	}
	return putImplementationEnvelope(record, envelope)
}

func nativeOrcaCurrentTurn(record *ExecutionRecord) *implementationTurnView {
	state, err := decodeOrcaExecutionState(record)
	if err != nil || state == nil || len(state.Turns) == 0 {
		return nil
	}
	return &state.Turns[len(state.Turns)-1]
}

func persistNativeOrcaTurn(record *ExecutionRecord, turn *implementationTurnView) error {
	if turn == nil {
		return errors.New("test native turn is required")
	}
	state, err := decodeOrcaExecutionState(record)
	if err != nil || state == nil {
		return errors.Join(err, errors.New("test native execution state is unavailable"))
	}
	for i := range state.Turns {
		if state.Turns[i].TurnID == turn.TurnID {
			state.Turns[i] = *turn
			return storeOrcaExecutionState(record, state)
		}
	}
	return errors.New("test native turn is not part of the fixed execution state")
}

func workerOrcaTerminalIdentity(identity executionapi.ProcessIdentity) orcaworker.TerminalIdentity {
	return orcaworker.TerminalIdentity{Kind: identity.Kind, Handle: identity.Terminal, RuntimeID: identity.RuntimeID, Incarnation: identity.Incarnation}
}

func (s *Service) readNativeOrcaOutput(ctx context.Context, run *Run, execution *ExecutionRecord, turn *implementationTurnView) error {
	err := s.readHostOutput(ctx, run)
	if err == nil && turn != nil {
		if current := nativeOrcaCurrentTurn(execution); current != nil {
			*turn = *current
		}
	}
	return err
}
