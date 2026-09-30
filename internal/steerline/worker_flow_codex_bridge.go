package steerline

import (
	"context"
	"errors"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
)

// The bridge supplies frozen SteerLine execution facts to the Codex adapter.
func codexAppServerCommand(c Config, endpoint string) (Command, error) {
	launch, err := codexapp.AppServerLaunch(c.Codex.Executable, c.Codex.Args, c.Profile, endpoint)
	if err != nil {
		return Command{}, err
	}
	return Command{Executable: launch.Executable, Args: launch.Args}, nil
}

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
