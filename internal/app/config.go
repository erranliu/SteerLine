package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	steerline "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	executionprocess "github.com/erranliu/SteerLine/internal/execution/process"
	"github.com/erranliu/SteerLine/internal/execution/workspace"
)

type ProjectInfo struct {
	Name       string
	ConfigPath string
	Repository string
	RunDir     string
	Err        error
}

func HomeDir() string {
	if value := strings.TrimSpace(os.Getenv("STEERLINE_HOME")); value != "" {
		return filepath.Clean(value)
	}
	if runtime.GOOS == "windows" {
		return `C:\SteerlineRuntime`
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".steerline")
	}
	return filepath.Join(os.TempDir(), "steerline")
}

func BinDir() string { return filepath.Join(HomeDir(), "bin") }

func EtcDir() string { return filepath.Join(HomeDir(), "etc") }

func GlobalExecutionConfigPath() string { return filepath.Join(HomeDir(), "execution.json") }

func StateRoot() string { return filepath.Join(HomeDir(), "state") }

// LoadConfig reads the project file and fixed execution catalog as one
// application configuration. The returned catalog belongs to this config
// instance and is reused by all of its entry paths.
func LoadConfig(path string) (execution.Config, error) { return loadConfig(path, nil, nil, true) }

func LoadQueryConfig(path string) (execution.Config, error) { return loadConfig(path, nil, nil, false) }

func LoadConfigWithDirectory(path string, directory *execution.ExecutionDirectory, host executionbackend.ManagedProcessHost) (execution.Config, error) {
	if directory == nil || host == nil {
		return execution.Config{}, errors.New("execution directory and process host are required")
	}
	return loadConfig(path, directory, host, true)
}

func loadConfig(path string, directory *execution.ExecutionDirectory, processHost executionbackend.ManagedProcessHost, requireSupport bool) (execution.Config, error) {
	config := execution.Config{Remote: "origin", TaskPoll: "10s", PRPoll: "30s", Heartbeat: "5s", CommandTimeout: "30s", ShutdownGrace: "10s",
		Git: executionbackend.Command{Executable: "git"}, ExecutableTypes: []string{"task", "bug", "feature", "chore"}, StallTimeout: "5m"}
	projectPath, err := filepath.Abs(path)
	if err != nil {
		return config, err
	}
	projectBytes, err := os.ReadFile(projectPath)
	if err != nil {
		return config, err
	}
	if err = decodeStrictJSON(projectBytes, &config); err != nil {
		return config, err
	}
	if config.TaskAuthority != "native" {
		return config, errors.New("task_authority must be explicitly set to native; Beads is not supported")
	}
	if config.WorkflowProtocol != executionmodel.Version {
		return config, errors.New("workflow_protocol must be worker-flow-v1; older configurations are unsupported and their state is preserved")
	}
	if err := config.ExecutionPolicy.Validate(); err != nil {
		return config, err
	}
	if err := config.ExecutionRequirements.Validate(); err != nil {
		return config, err
	}
	if err := config.PublicationPolicy.Validate(); err != nil {
		return config, err
	}
	if err := publication.ValidateNonSecret(config.PR, config.Git); err != nil {
		return config, err
	}
	globalPath := GlobalExecutionConfigPath()
	if config.Host != "" || config.Agent != "" || len(config.ExecutionProfiles) != 0 || config.DefaultExecutionProfile != "" {
		return config, fmt.Errorf("execution profiles belong in global execution config %s", globalPath)
	}
	globalBytes, err := os.ReadFile(globalPath)
	if err != nil {
		return config, fmt.Errorf("load global execution config %s: %w", globalPath, err)
	}
	var global struct {
		Version           int                          `json:"version"`
		ExecutionProfiles []execution.ExecutionProfile `json:"execution_profiles"`
		DefaultProfile    string                       `json:"default_execution_profile"`
	}
	if err := decodeStrictJSON(globalBytes, &global); err != nil {
		return config, fmt.Errorf("global execution config: %w", err)
	}
	if global.Version != 2 {
		return config, errors.New("global execution config version must be 2; older development formats are unsupported and their state must be preserved")
	}
	if len(global.ExecutionProfiles) == 0 {
		return config, errors.New("global execution config requires at least one execution profile")
	}
	if processHost == nil {
		processHost = NewProcessHost(positiveDuration(config.CommandTimeout, 30*time.Second))
	}
	if directory == nil {
		directory, err = NewBuiltinDirectory(processHost)
	}
	if err != nil {
		return config, err
	}
	config.ProcessHost = processHost
	config.ProcessFacts = executionprocess.NewServiceFacts()
	config.Resources = executionResources{}
	config.ReadRuntime = func(_ context.Context, c execution.Config) (*reconstructiontypes.Runtime, error) {
		return LoadDatabaseRuntime(c)
	}
	configureNativeStorage(&config)
	config.ExecutionDirectory = directory
	config.StateRoot = StateRoot()
	config.ExecutionProfiles = global.ExecutionProfiles
	config.DefaultExecutionProfile = global.DefaultProfile
	config.RepositoryStateKey = ""
	config.BindApplicationConfig(directory, projectPath, globalPath, loadedConfigDigest(projectBytes, globalBytes), true)
	if err := validateQueryProfiles(config); err != nil {
		return config, err
	}
	if requireSupport {
		if err := config.ValidateProfiles(); err != nil {
			return config, err
		}
	}
	initial := global.ExecutionProfiles[0]
	if global.DefaultProfile != "" {
		for _, profile := range global.ExecutionProfiles {
			if profile.ID == global.DefaultProfile {
				initial = profile
				break
			}
		}
	}
	config = config.ApplyExecutionProfile(initial)
	base := filepath.Dir(projectPath)
	if config.Repository == "" || config.Branch == "" {
		return config, errors.New("repository and branch are required")
	}
	if !filepath.IsAbs(config.Repository) {
		config.Repository = filepath.Join(base, config.Repository)
	}
	if config.RunDir == "" {
		key, err := repositoryStateKey(config, processHost)
		if err != nil {
			return config, fmt.Errorf("run_dir is required (cannot derive from repository identity: %w)", err)
		}
		config.RepositoryStateKey = key
		config.RunDir = filepath.Join(config.StateRoot, key)
	} else {
		if !filepath.IsAbs(config.RunDir) {
			config.RunDir = filepath.Join(base, config.RunDir)
		}
		key, err := repositoryStateKey(config, processHost)
		if err != nil {
			return config, err
		}
		config.RepositoryStateKey = key
	}
	if config.WorktreeRoot == "" {
		config.WorktreeRoot = filepath.Join(config.RunDir, "worktrees")
	} else if !filepath.IsAbs(config.WorktreeRoot) {
		config.WorktreeRoot = filepath.Join(base, config.WorktreeRoot)
	}
	for _, value := range []string{config.TaskPoll, config.PRPoll, config.Heartbeat, config.CommandTimeout, config.ShutdownGrace} {
		duration, err := time.ParseDuration(value)
		if err != nil || duration <= 0 {
			return config, fmt.Errorf("invalid positive duration %q", value)
		}
	}
	if config.StallTimeout != "" {
		if duration, err := time.ParseDuration(config.StallTimeout); err != nil || duration < 0 {
			return config, errors.New("stall_timeout must be zero or a positive duration")
		}
	}
	if len(config.ExecutableTypes) == 0 {
		return config, errors.New("executable_types is empty")
	}
	for _, value := range config.ExecutableTypes {
		if !regexp.MustCompile(`^[a-z][a-z0-9-]*$`).MatchString(value) || value == "epic" || value == "molecule" || value == "convoy" {
			return config, fmt.Errorf("invalid executable type %q", value)
		}
	}
	if requireSupport {
		if err := config.ValidateProfiles(); err != nil {
			return config, err
		}
	}
	if err := ValidateApplicationPaths(config); err != nil {
		return config, err
	}
	return config, nil
}

func validateQueryProfiles(config execution.Config) error {
	seen := map[string]bool{}
	for _, profile := range config.ExecutionProfiles {
		if !regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`).MatchString(profile.ID) || seen[profile.ID] || strings.TrimSpace(profile.Name) == "" {
			return errors.New("execution profile ID or name is invalid")
		}
		seen[profile.ID] = true

	}
	if config.DefaultExecutionProfile != "" && !seen[config.DefaultExecutionProfile] {
		return errors.New("default_execution_profile is not configured")
	}
	return nil
}

func repositoryStateKey(config execution.Config, processHost executionbackend.ManagedProcessHost) (string, error) {
	repository, err := workspace.ResolveRepository(context.Background(), workspace.Git{
		Command: executionbackend.LaunchCommand{Executable: config.Git.Executable, Args: append([]string(nil), config.Git.Args...)},
		Timeout: positiveDuration(config.CommandTimeout, 30*time.Second), Start: processHost.StartManaged,
	}, config.Repository)
	if err != nil {
		return "", err
	}
	if repository.CommonDir == "" {
		return "", errors.New("git returned an empty common directory")
	}
	return RepositoryStateKey(repository.CommonDir), nil
}

func canonicalPath(value string) string {
	path := filepath.Clean(value)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

func loadedConfigDigest(project, global []byte) string {
	digest := sha256.New()
	_, _ = digest.Write(project)
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(global)
	return hex.EncodeToString(digest.Sum(nil))
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:12])
}

func ListProjects() ([]ProjectInfo, error) {
	entries, err := os.ReadDir(EtcDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	projects := make([]ProjectInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(EtcDir(), entry.Name())
		info := ProjectInfo{Name: strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())), ConfigPath: path}
		config, loadErr := LoadQueryConfig(path)
		if loadErr != nil {
			info.Err = loadErr
		} else {
			info.Repository, info.RunDir = config.Repository, config.RunDir
		}
		projects = append(projects, info)
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	return projects, nil
}

func ResolveProject(projects []ProjectInfo, selected string) (ProjectInfo, error) {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return ProjectInfo{}, errors.New("no project selected")
	}
	if index := atoiSafe(selected); index >= 1 && index <= len(projects) {
		return projects[index-1], nil
	}
	for _, project := range projects {
		if strings.EqualFold(project.Name, selected) {
			return project, nil
		}
	}
	return ProjectInfo{}, fmt.Errorf("project %q not found", selected)
}

func atoiSafe(value string) int {
	number := 0
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return 0
		}
		number = number*10 + int(digit-'0')
	}
	return number
}

func InstallExecutable(source string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(source); err == nil {
		source = resolved
	}
	info, err := os.Stat(source)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", source)
	}
	name := "steerline"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.MkdirAll(BinDir(), 0700); err != nil {
		return "", err
	}
	destination := filepath.Join(BinDir(), name)
	if canonicalPath(source) == canonicalPath(destination) {
		return destination, nil
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return "", err
	}
	if err := steerline.AtomicWrite(destination, data); err != nil {
		return "", fmt.Errorf("install %s: %w (stop a running service using it first)", destination, err)
	}
	return destination, nil
}

func InstallSelf() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return InstallExecutable(executable)
}
