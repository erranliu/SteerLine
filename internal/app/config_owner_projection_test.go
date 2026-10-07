package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/execution"
	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func TestAppConfigProjectsOwnerInputsAndSelectedProfile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("STEERLINE_HOME", filepath.Join(root, "home"))
	repository := configTestRepository(t)
	fast := testExecutionProfile(t, "fast", "Fast", "local", "codex", codexworker.ProfileConfig{Executable: "codex-fast"}, nil)
	safe := testExecutionProfile(t, "safe", "Safe", "local", "codex", codexworker.ProfileConfig{Executable: "codex-safe"}, nil)
	writeGlobalExecutionFixture(t, []ExecutionProfile{fast, safe}, "fast")
	project := map[string]any{
		"repository": repository, "remote": "origin", "branch": "main",
		"task_authority": "native", "workflow_protocol": executionmodel.Version, "claimant": "service-claimant",
		"executable_types": []string{"task", "bug"}, "task_poll": "7s", "pr_poll": "17s", "heartbeat": "3s",
		"command_timeout": "9s", "shutdown_grace": "4s", "git": executionbackend.Command{Executable: "git"},
		"publication_policy": executionPublicationFixture(), "pr": hostPublicationProviderFixture(),
	}
	projectPath := filepath.Join(root, "project.json")
	raw, err := json.Marshal(project)
	require(t, err)
	require(t, os.WriteFile(projectPath, raw, 0600))

	config, err := LoadConfig(projectPath)
	require(t, err)
	require(t, config.ResolveIdentity(context.Background()))
	if config.projectPath == "" || config.executionPath != GlobalExecutionConfigPath() || config.configDigest == "" || config.ExecutionDirectory == nil || config.ProcessHost == nil {
		t.Fatal("app Config did not retain its loaded bytes, fixed catalog, and runtime ports")
	}
	if config.TaskAuthority != "native" || len(config.ExecutableTypes) != 2 || config.Claimant != "service-claimant" || config.TaskPoll != "7s" || config.PRPoll != "17s" || config.Heartbeat != "3s" {
		t.Fatal("app Config lost task or host owner inputs")
	}

	taskService := NewTaskService(nil, config)
	if taskService.Input.Authority != "native" || taskService.Input.DefaultRequestedBy == "" || strings.Join(taskService.Input.ExecutableTypes, ",") != "task,bug" {
		t.Fatalf("task projection does not own the native task inputs: %+v", taskService.Input)
	}
	hostConfig, err := BuildHostConfig(context.Background(), config)
	require(t, err)
	if hostConfig.Claimant != "service-claimant" || hostConfig.TaskPoll != "7s" || hostConfig.PRPoll != "17s" || hostConfig.Heartbeat != "3s" || hostConfig.ConfigDigest != config.LoadedConfigDigest() {
		t.Fatalf("host projection lost host-owned facts: %+v", hostConfig)
	}

	executionConfig := config.ExecutionConfig()
	serialized, err := json.Marshal(executionConfig)
	require(t, err)
	for _, appOnly := range []string{"task_authority", "execution_profiles", "default_execution_profile", "executable_types", "task_poll", "heartbeat", "shutdown_grace"} {
		if strings.Contains(string(serialized), appOnly) {
			t.Fatalf("frozen execution input contains app/task/host field %q: %s", appOnly, serialized)
		}
	}
	if executionConfig.Host != "local" || executionConfig.Agent != "codex" || executionConfig.Identity() == "" {
		t.Fatalf("frozen execution input lost selected route or project identity: %+v", executionConfig)
	}

	profiles := config.runtimeDependencies().Profiles
	if profiles.DefaultProfileID() != "fast" || !profiles.HasMultipleProfiles() {
		t.Fatalf("profile port lost its catalog default or profile count: default=%q multiple=%t", profiles.DefaultProfileID(), profiles.HasMultipleProfiles())
	}
	portType := reflect.TypeOf(appProfilePort{})
	catalogField, hasCatalog := portType.FieldByName("catalog")
	if portType.NumField() != 1 || !hasCatalog || catalogField.Type != reflect.TypeOf(execution.ProfileCatalog{}) {
		t.Fatalf("app profile port retains data outside its bounded resolver catalog: %v", portType)
	}
	defaultConfig, _, err := profiles.ResolveExecutionProfile("fast", "")
	require(t, err)

	defaultPayload, err := defaultConfig.DecodeImplementationConfig(codexworker.ProfileCodecKey, config.ExecutionDirectory)
	require(t, err)
	defaultSettings, err := codexworker.DecodeProfileConfig(defaultPayload)
	require(t, err)
	if defaultSettings.Executable != "codex-fast" {
		t.Fatalf("profile port did not select the configured default implementation: %+v", defaultSettings)
	}
	if _, _, err := profiles.ResolveExecutionProfile("fast", "stale-profile-digest"); err == nil || !strings.Contains(err.Error(), "stale_profile") {
		t.Fatalf("profile port accepted a stale profile digest: %v", err)
	}
	selected, fixed, err := profiles.ResolveExecutionProfile("safe", "")
	require(t, err)
	if fixed.ID != "safe" || fixed.Digest == "" || fixed.AcceptedConfigVersion != config.LoadedConfigDigest() || selected.Host != "local" {
		t.Fatalf("profile port did not return one exact frozen selection: config=%+v fixed=%+v", selected, fixed)
	}
	decoded, err := selected.DecodeImplementationConfig(codexworker.ProfileCodecKey, config.ExecutionDirectory)
	require(t, err)
	settings, err := codexworker.DecodeProfileConfig(decoded)
	require(t, err)
	if settings.Executable != "codex-safe" {
		t.Fatalf("selected profile reused another profile's implementation data: %+v", settings)
	}
	var wire executionmodel.FixedExecutionProfile
	if err := json.Unmarshal(mustJSON(fixed), &wire); err != nil || wire.Digest != fixed.Digest {
		t.Fatalf("fixed profile wire identity changed: %+v %v", wire, err)
	}
}
