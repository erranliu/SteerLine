package steerline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
)

const neutralProfileKey = "fixture.config"
const neutralBindingKey = "fixture.binding"
const neutralQuestionKey = "fixture.question"
const neutralPolicyKey = "fixture.policy"
const neutralProtocol executionapi.ExecutionProtocol = "fixture_stream_v1"

// The business fixture has one implementation and one codec. Its payload
// describes test input; it compiles no native command or permission mapping.
type neutralSettings struct {
	Executable      string                       `json:"executable"`
	Args            []string                     `json:"args,omitempty"`
	Model           string                       `json:"model,omitempty"`
	ReasoningEffort string                       `json:"reasoning_effort,omitempty"`
	InputMode       string                       `json:"input_mode,omitempty"`
	Target          executionapi.ExecutionTarget `json:"target,omitempty"`
}
type neutralBinding struct {
	Endpoint   string `json:"endpoint,omitempty"`
	Generation string `json:"generation,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
}

func neutralCodecs() []ImplementationProfileCodec {
	return []ImplementationProfileCodec{{Key: neutralProfileKey, Version: 1, Normalize: func(data []byte, policy executionapi.ExecutionPolicy, host, agent string) ([]byte, []string, error) {
		var value neutralSettings
		if err := decode(data, &value); err != nil {
			return nil, nil, err
		}
		if err := policy.Validate(); err != nil {
			return nil, nil, err
		}
		if value.Executable == "" {
			return nil, nil, errors.New("fixture executable is required")
		}
		display := []string{"neutral fixture; native permissions unverified"}
		if value.Model != "" {
			display = append(display, "Model："+value.Model)
		}
		if value.ReasoningEffort != "" {
			display = append(display, "Reasoning effort："+value.ReasoningEffort)
		}
		return mustJSON(value), display, nil
	}}}
}
func newNeutralDirectory(agents ...string) (*ExecutionDirectory, error) {
	key := ExecutionRegistrationKey{Backend: executionapi.BackendLocal, Protocol: neutralProtocol, Agent: "fixture"}
	entry := ExecutionRegistration{Key: key, ProfileKey: "fixture/stream-v1", Host: "local", WorkspaceBranchPrefix: "fixture", LocalWorkspace: true, ProfileCodecKeys: []string{neutralProfileKey}, QuestionCodecKey: neutralQuestionKey, QuestionCodecVersion: 1,
		Target: func(c Config) executionapi.ExecutionTarget {
			target := neutralProfileSettings(c).Target
			if target.ID == "" {
				target.ID = "fixture:fixed"
			}
			return target
		},
		Backend: func(c Config) executionapi.ExecutionBackend { return neutralBackend{config: c} },
		Lifecycle: func(setup executionapi.LifecycleSetup) (executionapi.LifecyclePort, error) {
			return &neutralLifecycle{setup: setup.Clone()}, nil
		},
		ProbeRequirements: func(_ context.Context, c Config) (executionapi.CapabilityReport, error) {
			return capabilityReport(key.Backend, key.Protocol, targetFor(c), agentFor(c), "fixture-version-1"), nil
		},
		QueryTool:      queryHostRequirementTool,
		ValidateConfig: func(context.Context, Config) error { return nil }, Preflight: func(context.Context, Config) error { return nil }, PrepareClaim: func(context.Context, Config) (ClaimPreparation, error) { return ClaimPreparation{}, nil }, ReadinessRequired: func(Config) bool { return false },
		BuildPolicyResource: neutralBuildPolicyResource, VerifyPolicyResource: neutralVerifyPolicyResource,
		LinkedWorktreeWriteArgs: func(context.Context, Config, string) ([]string, error) { return nil, nil }, ValidTurnID: func(value string) bool { return value != "" }, CleanupCapabilities: []string{"start", "observe", "stop", "workspace_inspection", "output_reading"},
		ValidateRef: func(ref executionapi.ExecutionRef) error {
			if ref.External.Kind == "os" && (ref.External.PID <= 0 || ref.External.Created == "" || ref.External.Executable == "") {
				return errors.New("fixture process identity is incomplete")
			}
			if ref.External.Kind != "os" && ref.External.Kind != "fixture" {
				return errors.New("fixture resource kind is unsupported")
			}
			return nil
		}, ValidatePublicationPolicy: func(policy executionapi.PublicationPolicy, provider publication.Config) error {
			if err := policy.Validate(); err != nil {
				return err
			}
			return provider.ValidateRemote(provider.BaseURL + "/" + provider.Repository + ".git")
		},
	}
	if len(agents) == 0 {
		agents = []string{"fixture"}
	}
	entries := make([]ExecutionRegistration, 0, len(agents))
	for _, agent := range agents {
		registration := entry
		registration.Key.Agent = agent
		if agent != "fixture" {
			registration.ProfileKey = "fixture/stream-v1/" + agent
		}
		entries = append(entries, registration)
	}
	return NewExecutionDirectory("fixture-directory-v1", neutralCodecs(), entries)
}
func neutralProfileSettings(c Config) neutralSettings {
	data, err := c.DecodeImplementationConfig(neutralProfileKey)
	if err != nil {
		return neutralSettings{}
	}
	var value neutralSettings
	_ = decode(data, &value)
	return value
}
func neutralProfile(t *testing.T, id, name string, value neutralSettings) ExecutionProfile {
	t.Helper()
	directory, err := newNeutralDirectory()
	require(t, err)
	return ExecutionProfile{directory: directory, ID: id, Name: name, Host: "local", Agent: "fixture", ImplementationConfigs: []executionapi.ImplementationConfigInput{{ImplementationKey: neutralProfileKey, CodecVersion: 1, Payload: mustJSON(value)}}}
}
func withNeutralProfile(c Config, value neutralSettings) Config {
	directory, err := newNeutralDirectory()
	if err != nil {
		panic(err)
	}
	c.ExecutionDirectory = directory
	c.ProcessHost = testProcessHost{}
	c.Host, c.Agent = "local", "fixture"
	if c.configDigest == "" {
		c.configDigest = "fixture-accepted-v1"
	}
	if c.WorktreeRoot == "" && c.RunDir != "" {
		c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	}
	if c.StateRoot == "" {
		c.StateRoot = fixtureStateRoot()
	}
	if c.Identity() != "" {
		c.RepositoryStateKey = RepositoryStateKey(c.Identity())
	}
	data, _, err := directory.NormalizeProfile([]executionapi.ImplementationConfigInput{{ImplementationKey: neutralProfileKey, CodecVersion: 1, Payload: mustJSON(value)}}, c.ExecutionPolicy, c.Host, c.Agent, configurationProjectIdentity(c), ProjectExecutionProfileID, c.configDigest)
	if err != nil {
		panic(err)
	}
	c.ImplementationData = data
	c.selectedProfileID = ProjectExecutionProfileID
	p := c.projectExecutionProfile()
	c.ImplementationData = cloneImplementationData(p.ImplementationData)
	c.ImplementationDisplay = append([]string(nil), p.ImplementationDisplay...)
	c.fixedProfileDigest = p.DigestFor(configurationProjectIdentity(c))
	c.WorkspaceBranchPrefix = "fixture"
	return c
}
func setNeutralProfile(t *testing.T, c *Config, value neutralSettings) {
	t.Helper()
	*c = withNeutralProfile(*c, value)
}
func neutralBuildPolicyResource(_ context.Context, r PolicyResourceBuildRequest) (PolicyResourceBuildResult, error) {
	e, err := executionapi.NewImplementationDataEnvelope(executionapi.ImplementationDataEnvelope{ImplementationKey: neutralPolicyKey, CodecVersion: 1, Purpose: executionapi.ImplementationResourcePurpose, ProjectID: r.Resource.ProjectID, ExecutionID: r.Resource.ExecutionID, AttemptID: r.Resource.AttemptID, Generation: r.Resource.Generation, ConfigDigest: r.Resource.ConfigDigest}, mustJSON(struct {
		Workspace string `json:"workspace"`
	}{r.Workspace}))
	return PolicyResourceBuildResult{Envelope: e, WriteRoots: []string{r.Workspace}}, err
}
func neutralVerifyPolicyResource(e executionapi.ImplementationDataEnvelope, id PolicyResourceIdentity) error {
	if e.ImplementationKey != neutralPolicyKey || e.CodecVersion != 1 {
		return errors.New("fixture policy codec differs")
	}
	return e.ValidateResourceIdentity(id.ProjectID, id.ExecutionID, id.AttemptID, id.Generation, id.ConfigDigest)
}
func setNeutralBinding(t *testing.T, e *ExecutionRecord, endpoint, generation, session string) {
	t.Helper()
	if e.ProjectIdentity == "" {
		e.ProjectIdentity = "test-project"
	}
	if e.ExecutionID == "" {
		e.ExecutionID = "test-execution"
	}
	if e.CurrentAttemptID == "" {
		e.CurrentAttemptID = "test-attempt"
	}
	if e.ConfigurationDigest == "" {
		e.ConfigurationDigest = "test-config-digest"
	}
	if e.Generation == "" {
		e.Generation = "test-resource-generation"
	}
	envelope, err := executionapi.NewImplementationDataEnvelope(executionapi.ImplementationDataEnvelope{ImplementationKey: neutralBindingKey, CodecVersion: 1, Purpose: executionapi.ImplementationResourcePurpose, ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest}, mustJSON(neutralBinding{endpoint, generation, session}))
	require(t, err)
	require(t, putImplementationEnvelope(e, envelope))
	e.SessionID, e.SessionGeneration = session, generation
}
func decodeNeutralBinding(envelope executionapi.ImplementationDataEnvelope, record *ExecutionRecord) (neutralBinding, error) {
	var value neutralBinding
	err := envelope.ValidateResourceIdentity(record.ProjectIdentity, record.ExecutionID, record.CurrentAttemptID, record.Generation, record.ConfigurationDigest)
	if err == nil {
		err = decode(envelope.Payload, &value)
	}
	return value, err
}
func neutralDecisionFixture(t *testing.T, fixed executionapi.SessionContext, id string, questions []executionapi.Question) DecisionRequest {
	t.Helper()
	for i := range questions {
		questions[i].ExecutionID, questions[i].AttemptID, questions[i].Generation, questions[i].TurnID = fixed.ExecutionID, fixed.AttemptID, fixed.Generation, fixed.CurrentTurnID
	}
	route := simpleQuestionRoute(t, neutralQuestionKey, fixed.ProjectID, fixed.WorkflowRunID, fixed.ExecutionID, fixed.AttemptID, fixed.Generation, fixed.CurrentTurnID, id)
	return decisionRequestFromEventFact(executionapi.EventFact{QuestionRoute: &route, Questions: questions})
}
func neutralPolicyReceipt(profile, path string, e *ExecutionRecord) *contracts.PolicyResources {
	if e.ProjectIdentity == "" {
		e.ProjectIdentity = "test-project"
	}
	if e.ExecutionID == "" {
		e.ExecutionID = "test-execution"
	}
	if e.CurrentAttemptID == "" {
		e.CurrentAttemptID = "test-attempt"
	}
	if e.Generation == "" {
		e.Generation = newID()
	}
	if e.ConfigurationDigest == "" {
		e.ConfigurationDigest = profile
	}
	result, err := neutralBuildPolicyResource(context.Background(), PolicyResourceBuildRequest{Workspace: path, Resource: PolicyResourceIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: profile}})
	if err != nil {
		panic(err)
	}
	r := &contracts.PolicyResources{Version: 3, ProfileDigest: profile, Workspace: path, WriteRoots: []string{path}, TemporaryRoots: []string{}, ImplementationData: result.Envelope}
	r.Digest = policyResourceDigest(*r)
	return r
}
func newNeutralService(c Config) (*Service, error) {
	if c.ExecutionDirectory == nil {
		c = withNeutralProfile(c, neutralSettings{Executable: os.Args[0]})
	}
	c.ProcessHost = testProcessHost{}
	var err error
	c, err = PrepareServiceConfig(c)
	if err != nil {
		return nil, err
	}
	db, err := OpenNativeTaskCommandStore(context.Background(), &c)
	if err != nil {
		return nil, err
	}
	service, err := NewService(c, c.ExecutionDirectory, c.ExecutionDirectory, neutralConversationSources{}, c.ProcessHost, db)
	if err != nil {
		_ = db.Close()
	}
	return service, err
}

func neutralVerifyFixedExecutionProfile(p FixedExecutionProfile) bool {
	d, err := newNeutralDirectory()
	return err == nil && verifyFixedExecutionProfile(p, d)
}

func neutralDirectoryWithReplacement(t *testing.T, d *ExecutionDirectory, replacement ExecutionRegistration) *ExecutionDirectory {
	t.Helper()
	entries := make([]ExecutionRegistration, 0, len(d.entries))
	for key, e := range d.entries {
		if key == replacement.Key {
			e = replacement
		}
		entries = append(entries, e)
	}
	clone, err := NewExecutionDirectory(d.Version(), neutralCodecs(), entries)
	require(t, err)
	return clone
}

func neutralProfileFromInputs(inputs []executionapi.ImplementationConfigInput) neutralSettings {
	for _, input := range inputs {
		if input.ImplementationKey == neutralProfileKey {
			var value neutralSettings
			_ = decode(input.Payload, &value)
			return value
		}
	}
	return neutralSettings{}
}

func neutralExecutionDirectory(t *testing.T) *ExecutionDirectory {
	t.Helper()
	d, err := newNeutralDirectory()
	require(t, err)
	return d
}
