// Package remotetest contains the single input snapshot for the explicit execution profiles for environment and remote L4 tests.
package remotetest

import (
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
)

const Discovery = "configuration.execution-profiles-orca"
const Progress = "testing.project-progress-orca-browser"
const Environment = "testing.project-environment-orca"
const LocalEnvironment = "testing.project-environment-local"

type Input struct {
	Profile         executionconfig.Profile `json:"profile"`
	ExecutionDigest string                  `json:"execution_digest"`
	Project         string                  `json:"project,omitempty"`
	Commit          string                  `json:"commit,omitempty"`
	ProjectConfig   json.RawMessage         `json:"project_config,omitempty"`
	L2Verification  string                  `json:"l2_verification,omitempty"`
	RoundSeconds    int                     `json:"round_seconds,omitempty"`
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
		if _, err := env.Key(p.Execution); err != nil {
			return nil, err
		}
	} else {
		if p.Execution.Adapter != "orca-terminal" || p.Execution.Target.Kind != "orca_environment" || !regexp.MustCompile(`^[a-fA-F0-9]{8}(?:-[a-fA-F0-9]{4}){3}-[a-fA-F0-9]{12}$`).MatchString(p.Execution.Target.Environment) || p.Workflow.ID != "development" || p.Workflow.Version != 2 {
			return nil, errors.New("remote L4 requires an explicit Orca environment and development/2")
		}
		var settings struct {
			Transport struct {
				Executable string `json:"executable"`
			} `json:"transport"`
			AgentLaunch struct {
				Executable string `json:"executable"`
				Model      string `json:"model"`
			} `json:"agent_launch"`
		}
		if json.Unmarshal(p.Execution.Settings, &settings) != nil || !filepath.IsAbs(settings.Transport.Executable) || settings.AgentLaunch.Executable == "" || settings.AgentLaunch.Model == "" {
			return nil, errors.New("remote profile requires explicit Orca executable and remote Agent executable/model")
		}
		if info, e := os.Stat(settings.Transport.Executable); e != nil || !info.Mode().IsRegular() {
			return nil, errors.New("configured Orca executable unavailable")
		}
	}
	in.ExecutionDigest = Digest(p.Execution)
	if id == Discovery {
		return in, nil
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
		if json.Unmarshal(in.ProjectConfig, &project) != nil || project.Branch == "" || id != LocalEnvironment && project.Branch != "AIRefactor" || project.TaskAuthority != "native" {
			return nil, errors.New("308 project config must select AIRefactor and explicit native task authority")
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
	document, err := show(env.DocumentPath)
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
	rules, err := show(env.ExecutionRulesPath)
	if err != nil {
		return nil, err
	}
	expectedRules, err := prompts.EnvironmentExecutionRules()
	if err != nil {
		return nil, err
	}
	if !prompts.SameRules(rules, expectedRules) {
		return nil, errors.New("pinned project environment execution rules differ from this build; maintain and commit the same-version material before live validation")
	}
	if id == LocalEnvironment {
		in.L2Verification, err = environmentL2Verification(show)
		if err != nil {
			return nil, err
		}
	}
	return in, nil
}

// These are the two existing project verifiers reused by the live driver.
// Fail before fixture creation for unknown or ambiguous report formats.
func environmentL2Verification(show func(string) ([]byte, error)) (string, error) {
	formats := []struct {
		name  string
		paths []string
	}{
		{"python", []string{"SteerlineFlow/testing/test-list.json", "tools/test-project.py", "tools/project_checks.py", "tools/steerline_reports.py"}},
		{"managed-powershell", []string{"SteerlineFlow/testing/test-list.json", "tools/run-tests.ps1", "tools/testing/ExecutionPlan.ps1", "tools/testing/ManagedReports.ps1"}},
	}
	found := ""
	for _, format := range formats {
		complete := true
		for _, path := range format.paths {
			raw, err := show(path)
			if err != nil || len(raw) == 0 {
				complete = false
				break
			}
		}
		if complete {
			if found != "" {
				return "", errors.New("ambiguous original project L2 verifier")
			}
			found = format.name
		}
	}
	if found == "" {
		return "", errors.New("local environment L4 requires an existing committed Python or managed PowerShell project L2 verifier")
	}
	return found, nil
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
