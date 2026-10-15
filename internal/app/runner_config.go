package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

// RunnerExecutionConfig is the only global execution format accepted after
// cutover. Settings are public Runner JSON; SteerLine has no adapter codecs.
type RunnerExecutionConfig struct {
	Version        int                    `json:"version"`
	Runner         runnerclient.Selection `json:"runner"`
	DefaultProfile string                 `json:"default_execution_profile"`
	Profiles       []RunnerProfile        `json:"execution_profiles"`
}
type RunnerProfile struct {
	ID        string                      `json:"id"`
	Name      string                      `json:"name"`
	Execution runnerclient.ExecutionInput `json:"execution"`
	Workflow  runnerclient.Workflow       `json:"workflow"`
}

func (p RunnerProfile) Digest() string {
	body, _ := json.Marshal(p)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
func (g RunnerExecutionConfig) Validate() error {
	if g.Version != 3 || len(g.Profiles) == 0 || g.Runner.Version == "" {
		return errors.New("execution config requires version 3, a pinned Runner release and profiles")
	}
	if err := runnerclient.VerifyExecutable(g.Runner.Executable, g.Runner.SHA256); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, p := range g.Profiles {
		if !regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`).MatchString(p.ID) || strings.TrimSpace(p.Name) == "" || seen[p.ID] || p.Execution.Adapter == "" || p.Execution.Target.Kind == "" || len(p.Execution.Settings) == 0 || !json.Valid(p.Execution.Settings) || p.Workflow.ID == "" || p.Workflow.Version <= 0 {
			return errors.New("Runner execution profile is incomplete or duplicated")
		}
		seen[p.ID] = true
	}
	if !seen[g.DefaultProfile] {
		return errors.New("default_execution_profile must select a configured Runner profile")
	}
	return nil
}
func (g RunnerExecutionConfig) Profile(id, digest string) (RunnerProfile, error) {
	if id == "" {
		id = g.DefaultProfile
	}
	for _, p := range g.Profiles {
		if p.ID == id {
			if digest != "" && p.Digest() != digest {
				return p, taskError("stale_profile", "Runner profile changed; refresh before starting")
			}
			return p, nil
		}
	}
	return RunnerProfile{}, taskError("profile_not_found", "selected Runner profile is not configured")
}

func (g RunnerExecutionConfig) taskProfiles(repository string, publication runnerclient.Publication) map[string]taskrunner.Profile {
	profiles := make(map[string]taskrunner.Profile, len(g.Profiles))
	for _, p := range g.Profiles {
		profiles[p.ID] = taskrunner.Profile{ID: p.ID, Name: p.Name, Input: runnerclient.RunInput{Repository: runnerclient.Repository{LocalPath: repository}, Execution: p.Execution, Workflow: p.Workflow, Publication: publication}}
	}
	return profiles
}

// startRunner fixes the release and validates Runner's published schemas before
// the app publishes ready or commits a task claim. Only this explicit service
// startup path starts the daemon; opening/checking a project stays read-only.
func (g RunnerExecutionConfig) startRunner(ctx context.Context, runDir string, timeout time.Duration, publication runnerclient.Publication) (runnerclient.Client, runnerclient.ServiceInfo, error) {
	if err := g.Validate(); err != nil {
		return runnerclient.Client{}, runnerclient.ServiceInfo{}, err
	}
	cli, version, err := g.Runner.Open(ctx, filepath.Join(runDir, "runner"), timeout)
	if err != nil {
		return cli, runnerclient.ServiceInfo{}, err
	}
	info, err := cli.StartService(ctx, version)
	if err != nil {
		return cli, info, err
	}
	catalog, err := cli.Catalog(ctx, version)
	if err == nil {
		err = catalog.ValidatePublication(publication)
	}
	if err == nil {
		for _, profile := range g.Profiles {
			if e := catalog.ValidateExecution(profile.Execution); e != nil {
				err = fmt.Errorf("Runner profile %s: %w", profile.ID, e)
				break
			}
			if e := catalog.ValidateWorkflow(profile.Workflow); e != nil {
				err = fmt.Errorf("Runner workflow %s: %w", profile.ID, e)
				break
			}
		}
	}
	return cli, info, err
}
