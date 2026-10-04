package execution

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
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

func ResolveCommand(c *executionbackend.Command) error {
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
