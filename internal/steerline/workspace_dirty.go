package steerline

import "context"

func inspectGitDirty(ctx context.Context, c Config, worktree string) (bool, error) {
	output, err := gitRunner(c, worktree).Output(ctx, c.Git, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return true, err
	}
	return len(output) != 0, nil
}
