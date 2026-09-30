package steerline

import (
	"errors"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/workspace"
)

var commitHash = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func (c *Config) prepareWorktreeRoot() error {
	if c.WorktreeRoot == "" {
		c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	}
	if c.Repository == "" || c.identity == "" {
		return errors.New("worktree root validation requires the configured repository identity")
	}
	repository := workspace.Repository{Path: c.Repository, CommonDir: c.identity}
	root, err := workspace.ValidateRoot(repository, c.WorktreeRoot, c.RunDir)
	if err != nil {
		return err
	}
	c.WorktreeRoot = root
	return nil
}

func planWorkspace(c Config, taskID, runID, base string) (*executionapi.WorkspaceIntent, error) {
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`).MatchString(taskID) || strings.Contains(taskID, "..") {
		return nil, errors.New("task ID cannot safely form a branch name")
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(runID) || !commitHash.MatchString(base) {
		return nil, errors.New("invalid workspace identity")
	}
	return &executionapi.WorkspaceIntent{Path: filepath.Join(c.WorktreeRoot, runID), Branch: "codex/" + taskID + "-" + runID[:12], BaseCommit: base}, nil
}

func workspaceGit(c Config) workspace.Git {
	timeout := duration(c.CommandTimeout)
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return workspace.Git{
		Command: executionapi.LaunchCommand{Executable: c.Git.Executable, Args: append([]string(nil), c.Git.Args...)},
		Timeout: timeout,
		Start:   func(cmd *exec.Cmd) (executionapi.StopHandle, error) { return startProcess(cmd) },
	}
}

func workspaceRepository(c Config) (workspace.Repository, error) {
	if c.identity == "" || c.Repository == "" || c.hostName() == "orca_supervised" {
		return workspace.Repository{}, errors.New("local Git workspace requires its fixed repository identity")
	}
	return workspace.Repository{Path: c.Repository, CommonDir: c.identity}, nil
}
