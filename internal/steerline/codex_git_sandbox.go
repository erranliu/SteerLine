package steerline

import (
	"context"
	"errors"
	"runtime"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/workspace"
)

// Windows workspace-write needs Git's shared objects, refs and reflogs, plus
// this linked worktree's admin directory to stage and commit from a Codex step.
func codexLinkedWorktreeWriteArgs(ctx context.Context, c Config, worktree string) ([]string, error) {
	if runtime.GOOS != "windows" || c.Sandbox != "workspace-write" {
		return nil, nil
	}
	if c.identity == "" {
		return nil, errors.New("Codex Git sandbox requires a resolved repository identity")
	}
	repository, err := workspaceRepository(c)
	if err != nil {
		return nil, err
	}
	resources, err := workspace.ResolveLinkedResourcesAt(ctx, workspaceGit(c), repository, worktree)
	if err != nil {
		return nil, err
	}
	return codexworker.LinkedWorktreeWriteArgs(resources.Objects, resources.Refs, resources.Logs, resources.Admin)
}
