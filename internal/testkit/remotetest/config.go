// Package remotetest contains the single input snapshot for the explicit execution profiles for environment and remote L4 tests.
package remotetest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	tm "github.com/erranliu/SteerLine/internal/testmanagement/model"
	progresstools "github.com/erranliu/SteerLine/tools"
)

const Discovery = "configuration.execution-profiles-orca"
const Progress = "testing.project-progress-orca-browser"
const Environment = "testing.project-environment-orca"
const LocalEnvironment = "testing.project-environment-local"

type Input struct {
	Profile           executionconfig.Profile  `json:"profile"`
	GenerationProfile *executionconfig.Profile `json:"generation_profile,omitempty"`
	ExecutionDigest   string                   `json:"execution_digest"`
	Project           string                   `json:"project,omitempty"`
	Commit            string                   `json:"commit,omitempty"`
	ProjectConfig     json.RawMessage          `json:"project_config,omitempty"`
	RoundSeconds      int                      `json:"round_seconds,omitempty"`
}

func Selected(id string) bool {
	return id == Discovery || id == Progress || id == Environment || id == LocalEnvironment
}

// Load is called once, by test-feedback before builds or fixture creation.
// Children receive this value in STEERLINE_E2E_RUN, never the source file path.
func Load(id string, getenv func(string) string) (*Input, error) {
	if !Selected(id) {
		return nil, nil
	}
	profileVariable := "STEERLINE_L4_REMOTE_PROFILE"
	if id == LocalEnvironment {
		profileVariable = "STEERLINE_L4_LOCAL_PROFILE"
	}
	path := getenv(profileVariable)
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("%s must be an absolute profile path", profileVariable)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	in := &Input{}
	if err = executionconfig.Decode(raw, &in.Profile); err != nil {
		return nil, err
	}
	p := in.Profile
	if err = executionconfig.Validate(3, p.ID, []executionconfig.Profile{p}); err != nil {
		return nil, err
	}
	if id == LocalEnvironment {
		if p.Execution.Target.Kind != "local" || p.Execution.Target.Environment != "" || (p.Execution.Adapter != "codex-app-server" && p.Execution.Adapter != "orca-terminal") || p.Workflow.ID != "development" || p.Workflow.Version != 2 {
			return nil, errors.New("local environment L4 requires an explicit local Codex/Orca profile and development/2")
		}
	} else {
		if p.Execution.Adapter != "orca-terminal" || p.Execution.Target.Kind != "orca_environment" || !regexp.MustCompile(`^[a-fA-F0-9]{8}(?:-[a-fA-F0-9]{4}){3}-[a-fA-F0-9]{12}$`).MatchString(p.Execution.Target.Environment) || p.Workflow.ID != "development" || p.Workflow.Version != 2 {
			return nil, errors.New("remote L4 requires an explicit Orca environment and development/2")
		}
	}

	var settings map[string]json.RawMessage
	if _, e := runnercontract.CanonicalJSON(p.Execution.Settings); e != nil {
		return nil, e
	}
	if json.Unmarshal(p.Execution.Settings, &settings) != nil || len(settings) == 0 {
		return nil, errors.New("explicit execution settings required; Runner validates native semantics before execution")
	}
	in.ExecutionDigest = Digest(p.Execution)
	if id == Discovery {
		return in, nil
	}
	if id == Environment || id == LocalEnvironment {
		if path := getenv("STEERLINE_L4_GENERATION_PROFILE"); path != "" {
			if !filepath.IsAbs(path) {
				return nil, errors.New("STEERLINE_L4_GENERATION_PROFILE must be an absolute explicit profile path")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			var profile executionconfig.Profile
			if err := executionconfig.Decode(raw, &profile); err != nil {
				return nil, err
			}
			if profile.Execution.Adapter != "codex-app-server" || profile.Execution.Target.Kind != "local" || profile.Execution.Target.Environment != "" || profile.Workflow.ID != "development" || profile.Workflow.Version != 2 {
				return nil, errors.New("admission material generation requires an explicit local Codex development/2 profile")
			}
			if err := executionconfig.Validate(3, p.ID, []executionconfig.Profile{p, profile}); err != nil {
				return nil, err
			}
			in.GenerationProfile = &profile
		}
	}
	in.RoundSeconds, err = strconv.Atoi(getenv("STEERLINE_L4_ROUND_TIMEOUT_SECONDS"))
	if err != nil || in.RoundSeconds <= 0 || int64(in.RoundSeconds) > int64((time.Duration(1<<63-1)-40*time.Minute)/3/time.Second) {
		return nil, errors.New("STEERLINE_L4_ROUND_TIMEOUT_SECONDS must be a positive non-overflowing number")
	}
	if id == Progress {
		in.Project, in.Commit = getenv("STEERLINE_PROGRESS_PROJECT"), getenv("STEERLINE_PROGRESS_COMMIT")
	} else {
		config := getenv("STEERLINE_L4_PROJECT_CONFIG")
		if !filepath.IsAbs(config) {
			return nil, errors.New("STEERLINE_L4_PROJECT_CONFIG must be an absolute project config path")
		}
		in.ProjectConfig, err = os.ReadFile(config)
		if err != nil {
			return nil, err
		}
		var project struct {
			Repository    string `json:"repository"`
			Branch        string `json:"branch"`
			TaskAuthority string `json:"task_authority"`
		}
		if json.Unmarshal(in.ProjectConfig, &project) != nil || project.Branch == "" || project.TaskAuthority != "native" {
			return nil, errors.New("project config must select an explicit branch and native task authority")
		}
		in.Project, in.Commit = project.Repository, getenv("STEERLINE_L4_PROJECT_COMMIT")
	}
	if !filepath.IsAbs(in.Project) || !regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`).MatchString(in.Commit) {
		return nil, errors.New("remote project source requires an absolute repository path and full Git SHA")
	}
	if st, err := os.Stat(in.Project); err != nil || !st.IsDir() {
		return nil, errors.New("remote project source directory unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	show := func(path string) ([]byte, error) {
		raw, e := exec.CommandContext(ctx, "git", "-C", in.Project, "show", in.Commit+":"+path).Output()
		if e != nil {
			return nil, fmt.Errorf("source %s lacks committed %s: %w", in.Commit, path, e)
		}
		return raw, nil
	}

	document, err := show(env.DocumentPath())
	if err != nil {
		return nil, err
	}
	manifest, err := env.ParseManifest(document)
	if err != nil {
		return nil, err
	}

	script, err := show(manifest.Script)
	if err != nil {
		return nil, err
	}
	if len(script) == 0 {
		return nil, errors.New("committed environment script is empty")
	}
	for _, path := range manifest.Dependencies {
		if _, err := show(path); err != nil {
			return nil, err
		}
	}
	if id != Progress {
		return in, nil
	}

	// The live harness also starts a Task admission probe. Its fixed testing
	// protocol/writer are test prerequisites, not environment requirement sources.
	testingRules, err := show(tm.ProtocolPath)
	if err != nil {
		return nil, fmt.Errorf("admission probe testing protocol: %w", err)
	}
	expectedTesting, err := prompts.TestingRules()
	if err != nil {
		return nil, err
	}
	if !prompts.SameRules(testingRules, expectedTesting) {
		return nil, errors.New("admission probe testing protocol differs from this build; adopt same-version material before live validation")
	}
	writer, err := show(tm.WriterPath)
	if err != nil {
		return nil, fmt.Errorf("admission probe writer: %w", err)
	}
	if !bytes.Equal(bytes.ReplaceAll(writer, []byte("\r\n"), []byte("\n")), bytes.ReplaceAll([]byte(progresstools.PowerShellWriter), []byte("\r\n"), []byte("\n"))) {
		return nil, errors.New("admission probe writer differs from this build")
	}

	return in, nil
}

func Digest(execution runnercontract.ExecutionInput) string {
	raw, _ := json.Marshal(execution)
	canonical, err := runnercontract.CanonicalJSON(raw)
	if err != nil {
		return ""
	}
	return executionconfig.Digest(canonical)
}

func (in *Input) Timeout(id string) time.Duration {

	if id == Discovery {
		return 15 * time.Minute
	}
	return 3*time.Duration(in.RoundSeconds)*time.Second + 30*time.Minute
}
