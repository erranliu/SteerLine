package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	"github.com/erranliu/SteerLine/internal/localservice"
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
		if resolved, err := resolveAbsolutePath(value); err == nil {
			return resolved
		}
		path, err := filepath.Abs(filepath.Clean(value))
		if err == nil {
			return path
		}
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
func LoadConfig(path string) (Config, error) { return loadConfig(path, nil, true) }

func LoadQueryConfig(path string) (Config, error) { return loadConfig(path, nil, false) }

func loadConfig(path string, processHost executionbackend.ManagedProcessHost, requireSupport bool) (Config, error) {
	projectBytes, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	globalBytes, err := os.ReadFile(GlobalExecutionConfigPath())
	if err != nil {
		return Config{}, err
	}
	return loadConfigSnapshot(path, projectBytes, globalBytes, processHost, requireSupport)
}
func loadConfigSnapshot(path string, projectBytes, globalBytes []byte, processHost executionbackend.ManagedProcessHost, requireSupport bool) (Config, error) {
	return loadConfigSnapshotWithQueries(path, projectBytes, globalBytes, processHost, requireSupport, nil)
}

func loadConfigSnapshotWithQueries(path string, projectBytes, globalBytes []byte, processHost executionbackend.ManagedProcessHost, requireSupport bool, queries *startupQueries) (Config, error) {
	input, err := DecodeProjectConfigInput(projectBytes)
	config := configFromProjectInput(input)
	if err != nil {
		return config, err
	}
	projectPath, err := filepath.Abs(path)
	if err != nil {
		return config, err
	}
	config.startup = queries
	config.ConfigPath = projectPath
	if config.TaskAuthority != "native" {
		return config, errors.New("task_authority must be explicitly set to native; Beads is not supported")
	}
	if config.WorkflowProtocol != executionmodel.Version {
		return config, errors.New("workflow_protocol must be worker-flow-v2; older configurations are unsupported and their state is preserved")
	}

	if err := config.PublicationPolicy.Validate(); err != nil {
		return config, err
	}
	if err := publication.ValidateNonSecret(publication.Config(input.PR), config.Git); err != nil {
		return config, err
	}
	globalPath := GlobalExecutionConfigPath()
	var global RunnerExecutionConfig
	if err := decodeStrictJSON(globalBytes, &global); err != nil {
		return config, fmt.Errorf("global execution config: %w", err)
	}
	if err := global.Validate(); err != nil {
		return config, err
	}
	if processHost == nil {
		processHost = NewProcessHost(positiveDuration(config.CommandTimeout, 30*time.Second))
	}
	config.ProcessHost = processHost
	config.RunnerExecution = global
	config.bindApplicationFiles(projectPath, globalPath, loadedConfigDigest(projectBytes, globalBytes), true)
	base := filepath.Dir(projectPath)
	if config.Repository == "" || config.Branch == "" {
		return config, errors.New("repository and branch are required")
	}
	if !filepath.IsAbs(config.Repository) {
		config.Repository = filepath.Join(base, config.Repository)
	}
	if config.RunDir == "" {
		key, err := repositoryStateKey(&config)
		if err != nil {
			return config, fmt.Errorf("run_dir is required (cannot derive from repository identity: %w)", err)
		}
		config.RunDir = filepath.Join(StateRoot(), key)
	} else {
		if !filepath.IsAbs(config.RunDir) {
			config.RunDir = filepath.Join(base, config.RunDir)
		}
		if _, err := repositoryStateKey(&config); err != nil {
			return config, err
		}
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
	if err := validateRuntimeHomePaths(config); err != nil {
		return config, err
	}
	if err := ValidateApplicationPaths(config); err != nil {
		return config, err
	}
	return config, nil
}

func validateQueryProfiles(config Config) error { return config.RunnerExecution.Validate() }

func repositoryStateKey(config *Config) (string, error) {
	if err := config.ResolveIdentity(context.Background()); err != nil {
		return "", err
	}
	if config.Identity() == "" {
		return "", errors.New("git returned an empty common directory")
	}
	return RepositoryStateKey(config.Identity()), nil
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
	name := "steerline"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	target, err := localservice.Install(HomeDir(), source, name)
	if err != nil {
		return "", err
	}
	if _, err = InstallLaunchers(); err != nil {
		return target, err
	}
	return target, nil
}

func InstallSelf() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return InstallExecutable(executable)
}
