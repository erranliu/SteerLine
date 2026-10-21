package app

import (
	"errors"

	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

// RunnerExecutionConfig is the only global execution format accepted after
// cutover. Settings are public Runner JSON; SteerLine has no adapter codecs.
type RunnerExecutionConfig executionconfig.File[runnerclient.Selection]
type RunnerProfile = executionconfig.Profile

func (g RunnerExecutionConfig) Validate() error {
	if g.Runner.Version == "" {
		return errors.New("Runner version is required")
	}
	if err := runnerclient.VerifyExecutable(g.Runner.Executable, g.Runner.SHA256); err != nil {
		return err
	}
	return executionconfig.Validate(g.Version, g.DefaultProfile, g.Profiles)
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
