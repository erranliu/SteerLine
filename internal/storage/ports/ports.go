// Package ports owns pure persistence declarations and narrow command ports.
// Implementation owners retain transactions and authoritative version checks.
package ports

import (
	"context"
	"errors"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

var ErrNotFound = errors.New("persistent record not found")

type RecordWrite[T any] struct {
	Before *T
	After  T
}

type Commands interface {
	EnqueueCommand(context.Context, CommandRecord) (CommandRecord, bool, error)
	AcceptDecisionAnswer(context.Context, CommandRecord, executionmodel.DecisionAnswer) (executionmodel.DecisionRecord, bool, error)
	UpdateCommand(context.Context, CommandRecord) error
	Commands(context.Context, string, string, int) ([]CommandRecord, error)
	ServiceCommands(context.Context, string, string, int) ([]CommandRecord, error)
	Command(context.Context, string, string) (CommandRecord, error)
	CommandByIdempotency(context.Context, string, string) (CommandRecord, error)
	LookupCommand(context.Context, string, string) (CommandRecord, bool, error)
}

var ErrAnswerConflict = errors.New("answer_conflict: this decision already has a different accepted answer")
