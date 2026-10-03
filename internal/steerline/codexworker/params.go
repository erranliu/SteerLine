package codexworker

import (
	"context"
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func ThreadParams(worktree, sandbox, model string, dynamicTools []any) (map[string]any, error) {
	return ThreadStartParams(worktree, sandbox, model, dynamicTools)
}

type TurnParamsInput struct {
	Worktree string
	ThreadID string
	executionapi.TurnInput
	Sandbox         string
	Model           string
	ReasoningEffort string
}

func TurnParams(ctx context.Context, input TurnParamsInput, linkedWorktreeArgs func(context.Context, string) ([]string, error)) (map[string]any, error) {
	if !nativeID.MatchString(input.ThreadID) || !executionapi.StableExecutionID(input.CommandID) || input.Worktree == "" || input.Prompt == "" {
		return nil, errors.New("Codex app-server turn lacks a fixed thread, command or input")
	}
	var roots []string
	if input.Sandbox == "workspace-write" {
		args, err := linkedWorktreeArgs(ctx, input.Worktree)
		if err != nil {
			return nil, err
		}
		roots, err = WritableRoots(args)
		if err != nil {
			return nil, err
		}
	}
	return TurnStartParams(input.Worktree, input.ThreadID, input.CommandID, input.Prompt, input.Sandbox, input.Model, input.ReasoningEffort, roots)
}
