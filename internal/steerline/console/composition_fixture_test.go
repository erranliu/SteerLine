package console

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/processhost"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
)

// The console's unit fixtures supply public summaries and ports. Actual
// configuration loading and built-in support are covered by steerlineapp.
const consoleFixtureCodec = "fixture.public-config"

func consoleFixtureProfile(id, name, host, agent string, display []string) steerline.ExecutionProfile {
	payload, _ := json.Marshal(display)
	return steerline.ExecutionProfile{ID: id, Name: name, Host: host, Agent: agent, ImplementationConfigs: []executionapi.ImplementationConfigInput{{ImplementationKey: consoleFixtureCodec, CodecVersion: 1, Payload: payload}}}
}

func consoleFixtureConfig(c steerline.Config, path string) steerline.Config {
	if c.Host == "" {
		c.Host = "local"
	}
	if c.Agent == "" {
		c.Agent = "codex"
	}
	if c.ExecutionPolicy.Filesystem.ReadScope == "" {
		c.ExecutionPolicy = executionapi.ExecutionPolicy{Filesystem: executionapi.FilesystemPolicy{ReadScope: "host", WriteScope: "workspace"}, Network: "allow", Escalation: "deny"}
	}
	if c.PublicationPolicy.Publisher == "" {
		c.PublicationPolicy.Publisher = executionapi.PublisherHostService
	}
	codec := steerline.ImplementationProfileCodec{Key: consoleFixtureCodec, Version: 1, Normalize: func(data []byte, _ executionapi.ExecutionPolicy, _, _ string) ([]byte, []string, error) {
		var display []string
		if err := json.Unmarshal(data, &display); err != nil {
			return nil, nil, err
		}
		return append([]byte(nil), data...), display, nil
	}}
	protocol := executionapi.ExecutionProtocol("local_process")
	backend := executionapi.BackendKind("local")
	if c.Host != "local" {
		backend = "fixture-host"
		protocol = "fixture-protocol"
	}
	entry := steerline.ExecutionRegistration{Key: steerline.ExecutionRegistrationKey{Backend: backend, Protocol: protocol, Agent: c.Agent}, Host: c.Host, ProfileKey: "fixture/" + c.Host + "/" + c.Agent, WorkspaceBranchPrefix: "fixture", LocalWorkspace: true, ProfileCodecKeys: []string{consoleFixtureCodec}, QuestionCodecKey: "fixture.question", QuestionCodecVersion: 1,
		Backend: func(steerline.Config) executionapi.ExecutionBackend { return nil }, Target: func(steerline.Config) executionapi.ExecutionTarget {
			return executionapi.ExecutionTarget{ID: "fixture-target"}
		}, ProbeRequirements: func(context.Context, steerline.Config) (executionapi.CapabilityReport, error) {
			return executionapi.CapabilityReport{}, errors.New("fixture has no live target")
		}, QueryTool: func(context.Context, steerline.Config, string) executionapi.RequirementEvidence {
			return executionapi.RequirementEvidence{}
		}, Lifecycle: func(executionapi.LifecycleSetup) (executionapi.LifecyclePort, error) {
			return nil, errors.New("fixture does not launch")
		}, ValidateConfig: func(context.Context, steerline.Config) error { return nil }, Preflight: func(context.Context, steerline.Config) error { return nil }, PrepareClaim: func(context.Context, steerline.Config) (steerline.ClaimPreparation, error) {
			return steerline.ClaimPreparation{}, nil
		}, ReadinessRequired: func(steerline.Config) bool { return false }, BuildPolicyResource: func(context.Context, steerline.PolicyResourceBuildRequest) (steerline.PolicyResourceBuildResult, error) {
			return steerline.PolicyResourceBuildResult{}, errors.New("fixture does not build resources")
		}, VerifyPolicyResource: func(executionapi.ImplementationDataEnvelope, steerline.PolicyResourceIdentity) error { return nil }, LinkedWorktreeWriteArgs: func(context.Context, steerline.Config, string) ([]string, error) { return nil, nil }, ValidTurnID: func(string) bool { return true }, CleanupCapabilities: []string{"observe"}, ValidateRef: func(executionapi.ExecutionRef) error { return nil }, ValidatePublicationPolicy: func(p executionapi.PublicationPolicy, _ publication.Config) error { return p.Validate() }}
	d, err := steerline.NewExecutionDirectory("fixture-v1", []steerline.ImplementationProfileCodec{codec}, []steerline.ExecutionRegistration{entry})
	if err != nil {
		panic(err)
	}
	c.ProcessHost = processhost.New(30 * time.Second)
	digest := sha256.Sum256([]byte(path))
	c.BindApplicationConfig(d, path, "", hex.EncodeToString(digest[:]), false)
	display := c.ImplementationDisplay
	if len(display) == 0 {
		display = []string{"Model：未在全局配置固定", "Reasoning effort：high", "Sandbox：workspace-write"}
	}
	c = c.ApplyExecutionProfile(consoleFixtureProfile(steerline.ProjectExecutionProfileID, "Fixture", c.Host, c.Agent, display))
	return c
}

type consoleFixtureSource struct{ home string }

func (s consoleFixtureSource) ApplicationPaths() (string, string, string) {
	return s.home, filepath.Join(s.home, "bin"), filepath.Join(s.home, "etc")
}
func (s consoleFixtureSource) NormalizerSelector(first []byte) steerline.Normalizer {
	return steerline.DetectNormalizer(first, steerline.NewPiNormalizer(), codexworker.NewNormalizer())
}
func (s consoleFixtureSource) ConfigFiles() ([]ProjectConfigFile, error) {
	entries, err := os.ReadDir(filepath.Join(s.home, "etc"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []ProjectConfigFile
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			out = append(out, ProjectConfigFile{Name: strings.TrimSuffix(entry.Name(), ".json"), Path: filepath.Join(s.home, "etc", entry.Name())})
		}
	}
	return out, nil
}
func (s consoleFixtureSource) LoadConfig(path string) (steerline.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return steerline.Config{}, err
	}
	c := steerline.Config{Git: steerline.Command{Executable: "git"}, Remote: "origin", CommandTimeout: "30s"}
	if err := contracts.DecodeJSON(data, &c); err != nil {
		return c, err
	}
	c = consoleFixtureConfig(c, path)
	if err := c.ResolveIdentity(context.Background()); err != nil {
		return c, err
	}
	common := c.Identity()
	name := filepath.Base(common)
	if strings.EqualFold(name, ".git") {
		name = filepath.Base(filepath.Dir(common))
	}
	normalized := filepath.Clean(common)
	if runtime.GOOS == "windows" {
		normalized = strings.ToLower(normalized)
	}
	sum := sha256.Sum256([]byte(normalized))
	c.StateRoot = filepath.Join(s.home, "state")
	c.RepositoryStateKey = name + "-" + hex.EncodeToString(sum[:6])
	c.RunDir = filepath.Join(c.StateRoot, c.RepositoryStateKey)
	return c, nil
}
