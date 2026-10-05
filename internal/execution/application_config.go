package execution

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/workspace"
)

// LoadedConfigDigest identifies the project and global execution file bytes
// used to build this value. A console compares it with a running service before
// presenting execution settings as current.
func (c Config) LoadedConfigDigest() string { return c.configDigest }

// BindApplicationConfig attaches the selected application directory and the
// exact files used to load this configuration. It contains no loaded native
// settings; those remain in the implementation envelopes on the profiles.
func (c *Config) BindApplicationConfig(directory *ExecutionDirectory, projectPath, directoryPath, digest string, global bool) {
	c.ExecutionDirectory = directory
	c.policySource = projectPath
	c.executionConfigPath = directoryPath
	c.configDigest = digest
	c.globalExecution = global
}

func (c Config) Identity() string { return c.identity }

// ResolveIdentity reads only Git metadata and does not run task-provider probes
// or create runtime directories. Control clients use it without constructing a
// Service or taking ownership of scheduler state.
func (c *Config) ResolveIdentity(ctx context.Context) error {

	if err := ResolveCommand(&c.Git); err != nil {
		return err
	}
	if c.PR.Kind == "command" {
		for _, command := range []*executionbackend.Command{&c.PR.Query, &c.PR.Close} {
			if command.Executable != "" {

				_ = ResolveCommand(command)
			}
		}
	}
	repository, err := workspace.ResolveRepository(ctx, workspaceGit(*c), c.Repository)
	if err != nil {
		return err
	}
	identity := canonical(repository.CommonDir)
	if c.identity != "" && canonical(c.identity) != identity {
		return errors.New("configured repository identity changed after it was resolved")
	}
	c.identity = identity
	if c.frozenExecutionConfig {
		return validateImplementationProfileData(*c)
	}
	if c.selectedProfileID != "" {
		var selected ExecutionProfile
		found := false
		if c.globalExecution {
			for _, profile := range c.ExecutionProfiles {
				if profile.ID == c.selectedProfileID {
					selected, found = profile, true
					break
				}
			}
		} else if c.selectedProfileID == ProjectExecutionProfileID {
			selected, found = c.projectExecutionProfile(), true
		}
		if !found {
			return errors.New("selected execution profile is not available after project identity resolution")
		}
		*c = c.ApplyExecutionProfile(c.bindProjectPolicy(selected))
	}
	return nil
}

func inside(root, path string) bool {
	rel, e := filepath.Rel(root, path)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
