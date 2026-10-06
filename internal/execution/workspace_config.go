package execution

import (
	"errors"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/workspace"
)

var commitHash = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func (c *Config) PrepareWorktreeRoot() error {
	if c.WorktreeRoot == "" {
		return errors.New("explicit worktree_root is required")
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

func planWorkspace(c Config, taskID, runID, base string) (*executionbackend.WorkspaceIntent, error) {
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`).MatchString(taskID) || strings.Contains(taskID, "..") {
		return nil, errors.New("task ID cannot safely form a branch name")
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(runID) || !commitHash.MatchString(base) {
		return nil, errors.New("invalid workspace identity")
	}
	if c.WorkspaceBranchPrefix == "" {
		return nil, errors.New("execution directory did not provide a workspace branch prefix")
	}
	return &executionbackend.WorkspaceIntent{Path: filepath.Join(c.WorktreeRoot, runID), Branch: c.WorkspaceBranchPrefix + "/" + taskID + "-" + runID[:12], BaseCommit: base}, nil
}

func workspaceGit(c Config, processHost executionbackend.ManagedProcessHost) workspace.Git {
	timeout := duration(c.CommandTimeout)
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	start := func(cmd *exec.Cmd) (executionbackend.StopHandle, error) {
		if processHost == nil {
			return nil, errors.New("managed process host is required for workspace Git")
		}
		return processHost.StartManaged(cmd)
	}
	return workspace.Git{
		Command: executionbackend.LaunchCommand{Executable: c.Git.Executable, Args: append([]string(nil), c.Git.Args...)},
		Timeout: timeout,
		Start:   start,
	}
}

func workspaceRepository(c Config, directory *ExecutionDirectory) (workspace.Repository, error) {
	entry, err := registrationFor(c, directory)
	if err != nil || !entry.LocalWorkspace || c.identity == "" || c.Repository == "" {
		return workspace.Repository{}, errors.New("local Git workspace requires its fixed repository identity")
	}
	return workspace.Repository{Path: c.Repository, CommonDir: c.identity}, nil
}
