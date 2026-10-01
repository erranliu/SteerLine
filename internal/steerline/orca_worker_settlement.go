package steerline

import (
	"context"
	"errors"
)

type nativeOrcaSettlement struct {
	Outcome    string
	DispatchID string
	RuntimeID  string
}

func readNativeOrcaSettlement(ctx context.Context, c Config, execution *ExecutionRecord, turn *OrcaNativeTurn) (nativeOrcaSettlement, error) {
	var fact nativeOrcaSettlement
	if execution == nil || execution.OrcaNative == nil || execution.Process == nil || turn == nil {
		return fact, errors.New("worker settlement has no fixed execution identity")
	}
	ops, err := orcaOperationsFor(c)
	if err != nil {
		return fact, err
	}
	return ops.Settlement(ctx, execution, turn)
}

func verifyNativeOrcaActiveDispatch(ctx context.Context, c Config, execution *ExecutionRecord, turn *OrcaNativeTurn) error {
	if execution == nil || execution.Process == nil || turn == nil {
		return errors.New("active dispatch verification has no fixed identity")
	}
	ops, err := orcaOperationsFor(c)
	if err != nil {
		return err
	}
	return ops.VerifyActive(ctx, execution, turn)
}
