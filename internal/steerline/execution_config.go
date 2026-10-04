package steerline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
	"github.com/erranliu/SteerLine/internal/steerline/workspace"
)

func (c Config) ValidateProfiles() error { return validateExecutionProfiles(c) }

func validateWorkflowConfig(c *Config, _ []byte) error {
	entry, err := registrationFor(*c)
	if err != nil {
		return err
	}
	if err := c.ExecutionRequirements.Validate(); err != nil {
		return err
	}
	if err := entry.ValidatePublicationPolicy(c.PublicationPolicy, c.PR); err != nil {
		return err
	}
	if err := publication.ValidateNonSecret(c.PR, c.Git); err != nil {
		return err
	}
	if c.WorkflowProtocol != executionmodel.Version {
		return errors.New("only worker-flow-v1 is supported")
	}
	return validateImplementationProfileData(*c)
}

func duration(s string) time.Duration { d, _ := time.ParseDuration(s); return d }

func positiveDuration(value string, fallback time.Duration) time.Duration {
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

// hostName and agentName expose configured implementation identity values.
func (c Config) hostName() string {
	return c.Host
}

func (c Config) agentName() string {
	return c.Agent
}

func resolveCommand(c *executionbackend.Command) error {
	p, e := exec.LookPath(c.Executable)
	if e != nil {
		return e
	}
	ext := strings.ToLower(filepath.Ext(p))
	if ext == ".cmd" || ext == ".bat" || ext == ".ps1" {
		return fmt.Errorf("use a native executable (or node.exe with a script argument), not a shell shim: %s", p)
	}
	c.Executable = p
	return nil
}

func (c *Config) Preflight(ctx context.Context, checkSkills bool) error {
	if err := requireNativeTaskAuthority(*c); err != nil {
		return err
	}
	return c.preflightNative(ctx)
}

// preflightNative validates a native project without resolving or invoking a
// Beads executable. A check never initializes a database or adopts an existing
// state directory.
func (c *Config) preflightNative(ctx context.Context) error {
	if err := validateWorkflowConfig(c, nil); err != nil {
		return err
	}
	if err := requireNativeTaskAuthority(*c); err != nil {
		return err
	}
	if err := validateExecutionProfiles(*c); err != nil {
		return err
	}
	entry, err := registrationFor(*c)
	if err != nil {
		return err
	}
	if entry.ValidateConfig != nil {
		if err := entry.ValidateConfig(ctx, *c); err != nil {
			return err
		}
	}
	commands := []*executionbackend.Command{&c.Git}
	for _, command := range commands {
		if err := resolveCommand(command); err != nil {
			return err
		}
	}
	git := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	repository, err := workspace.ResolveRepository(ctx, workspaceGit(*c), c.Repository)
	if err != nil {
		return err
	}
	c.Repository = repository.Path
	// Rebind accepted profile envelopes to the same Git identity used by
	// control clients before implementation preflight decodes them.
	if err := c.ResolveIdentity(ctx); err != nil {
		return err
	}
	git.Dir = c.Repository
	if entry.Preflight != nil {
		if err := entry.Preflight(ctx, *c); err != nil {
			return err
		}
	}
	if c.Claimant == "" {
		c.Claimant = "steerline-" + shortHash(c.identity)
	}
	if _, err = git.Output(ctx, c.Git, "check-ref-format", "refs/heads/"+c.Branch); err != nil {
		return err
	}
	remote, err := git.Output(ctx, c.Git, "remote", "get-url", c.Remote)
	if err != nil {
		return err
	}
	if err = c.PR.ValidateRemote(strings.TrimSpace(string(remote))); err != nil {
		return err
	}
	if c.PR.Kind == "command" {
		if err = resolveCommand(&c.PR.Query); err != nil {
			return err
		}
		if c.PR.Close.Executable == "" {
			return errors.New("native command PR provider requires a close command for discarded task cleanup")
		}
		if err = resolveCommand(&c.PR.Close); err != nil {
			return err
		}
	}
	if err = ensureRunDirOutsideRepository(*c); err != nil {
		return err
	}
	if _, err = os.Stat(filepath.Join(c.RunDir, "steerline.db")); err == nil {
		reader, openErr := openBoundNativeStore(ctx, *c, filepath.Join(c.RunDir, "steerline.db"))
		if openErr != nil {
			return openErr
		}
		if closeErr := reader.Close(); closeErr != nil {
			return closeErr
		}
	} else if !os.IsNotExist(err) {
		return err
	} else if err = ensureNativeStateClean(c.RunDir, c.Repository); err != nil {
		return err
	}
	if c.PR.Kind == "gitea" {
		if err = (publication.NewClient(c.PR, git, git.Timeout)).Check(ctx); err != nil {
			return err
		}
	}
	return nil
}

type LegacyCompletionConfig = contracts.LegacyCompletionConfig
