package steerline

import (
	"context"
	"errors"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
)

// The bridge supplies frozen SteerLine execution facts to the Codex adapter.
func codexAppThreadStart(c Config, worktree string, dynamicTools []any) (map[string]any, error) {
	return codexapp.ThreadStartParams(worktree, c.Sandbox, c.Model, dynamicTools)
}

func codexAppTurnStart(ctx context.Context, c Config, worktree, threadID, commandID, prompt string) (map[string]any, error) {
	if !codexThreadID.MatchString(threadID) || !stableExecutionID(commandID) || worktree == "" || prompt == "" {
		return nil, errors.New("Codex app-server turn lacks a fixed thread, command or input")
	}
	var roots []string
	if c.Sandbox == "workspace-write" {
		args, err := codexLinkedWorktreeWriteArgs(ctx, c, worktree)
		if err != nil {
			return nil, err
		}
		roots, err = codexapp.WritableRoots(args)
		if err != nil {
			return nil, err
		}
	}
	return codexapp.TurnStartParams(worktree, threadID, commandID, prompt, c.Sandbox, c.Model, c.ReasoningEffort, roots)
}
