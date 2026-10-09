package execution

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
)

const neutralProfileKey = "fixture.config"

const neutralBindingKey = "fixture.binding"

const neutralQuestionKey = "fixture.question"

const neutralPolicyKey = "fixture.policy"

const neutralProtocol executionbackend.ExecutionProtocol = "fixture_stream_v1"

// The business fixture has one implementation and one codec. Its payload
// describes test input; it compiles no native command or permission mapping.
type neutralSettings struct {
	Executable      string                           `json:"executable"`
	Args            []string                         `json:"args,omitempty"`
	Model           string                           `json:"model,omitempty"`
	ReasoningEffort string                           `json:"reasoning_effort,omitempty"`
	InputMode       string                           `json:"input_mode,omitempty"`
	Target          executionbackend.ExecutionTarget `json:"target,omitempty"`
}

type neutralBinding struct {
	Endpoint   string `json:"endpoint,omitempty"`
	Generation string `json:"generation,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
}

func neutralCodecs() []ImplementationProfileCodec {
	return []ImplementationProfileCodec{{Key: neutralProfileKey, Version: 1, Normalize: func(data []byte, host, agent string) ([]byte, []string, error) {
		var value neutralSettings
		if err := decode(data, &value); err != nil {
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
	var directory *ExecutionDirectory
	key := ExecutionRegistrationKey{Backend: executionbackend.BackendLocal, Protocol: neutralProtocol, Agent: "fixture"}
	entry := ExecutionRegistration{ServiceStopBehavior: executionbackend.StopExecutionOnServiceStop, Key: key, ProfileKey: "fixture/stream-v1", Host: "local", WorkspaceBranchPrefix: "fixture", LocalWorkspace: true, ProfileCodecKeys: []string{neutralProfileKey}, QuestionCodecKey: neutralQuestionKey, QuestionCodecVersion: 1,
		Target: func(c Config) executionbackend.ExecutionTarget {
			target := neutralProfileSettings(c).Target
			if target.ID == "" {
				target.ID = "fixture:fixed"
			}
			return target
		},
		Backend: func(c Config) executionbackend.ExecutionBackend { return newNeutralBackend(c) },
		Lifecycle: func(setup executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
			return &neutralLifecycle{Setup: setup.Clone()}, nil
		},

		ValidateConfig: func(context.Context, Config) error { return nil }, Preflight: func(context.Context, Config) error { return nil }, PrepareClaim: func(context.Context, Config) (ClaimPreparation, error) { return ClaimPreparation{}, nil }, ReadinessRequired: func(Config) bool { return false },
		BuildPolicyResource: neutralBuildPolicyResource, VerifyPolicyResource: neutralVerifyPolicyResource,
		LinkedWorktreeWriteArgs: func(context.Context, Config, string) ([]string, error) { return nil, nil }, ValidTurnID: func(value string) bool { return value != "" }, CleanupCapabilities: []string{"start", "observe", "stop", "workspace_inspection", "output_reading"},
		ValidateRef: func(ref executionbackend.ExecutionRef) error {
			if ref.External.Kind == "os" && (ref.External.PID <= 0 || ref.External.Created == "" || ref.External.Executable == "") {
				return errors.New("fixture process identity is incomplete")
			}
			if ref.External.Kind != "os" && ref.External.Kind != "fixture" {
				return errors.New("fixture resource kind is unsupported")
			}
			return nil
		}, ValidatePublicationPolicy: func(policy executionbackend.PublicationPolicy, provider publication.Config) error {
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
	var err error
	directory, err = NewExecutionDirectory("fixture-directory-v1", neutralCodecs(), entries)
	return directory, err
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
	return ExecutionProfile{directory: directory, ID: id, Name: name, Host: "local", Agent: "fixture", ImplementationConfigs: []executionbackend.ImplementationConfigInput{{ImplementationKey: neutralProfileKey, CodecVersion: 1, Payload: mustJSON(value)}}}
}

func withNeutralProfile(c Config, value neutralSettings) Config {
	if c.Host == "" {
		c.Host = "local"
	}
	if c.Agent == "" {
		c.Agent = "fixture"
	}
	directory, err := newNeutralDirectory()
	if err != nil {
		panic(err)
	}
	profile := ExecutionProfile{directory: directory, ID: ProjectExecutionProfileID, Name: "Fixture", Host: c.Host, Agent: c.Agent,
		ImplementationConfigs: []executionbackend.ImplementationConfigInput{{ImplementationKey: neutralProfileKey, CodecVersion: 1, Payload: mustJSON(value)}}}
	return withNeutralProfiles(c, []ExecutionProfile{profile}, ProjectExecutionProfileID)
}

func withNeutralProfiles(c Config, profiles []ExecutionProfile, defaultID string) Config {
	var directory *ExecutionDirectory
	if len(profiles) > 0 {
		directory = profiles[0].directory
	}
	if directory == nil {
		var err error
		directory, err = newNeutralDirectory()
		if err != nil {
			panic(err)
		}
	}
	if c.Host == "" {
		c.Host = "local"
	}
	if c.Agent == "" {
		c.Agent = "fixture"
	}
	if c.acceptedConfigVersion == "" {
		c.acceptedConfigVersion = "fixture-accepted-v1"
	}
	if c.WorktreeRoot == "" && c.RunDir != "" {
		c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	}
	catalog := ProfileCatalog{Base: c, Directory: directory, Profiles: append([]ExecutionProfile(nil), profiles...), DefaultID: defaultID, Global: true}
	selectedID := defaultID
	if selectedID == "" && len(profiles) == 1 {
		selectedID = profiles[0].ID
	}
	if selectedID == "" && len(profiles) > 0 {
		selectedID = profiles[0].ID
	}
	selected, _, err := catalog.ResolveExecutionProfile(selectedID, "")
	if err != nil {
		panic(err)
	}
	rememberNeutralCatalog(selected, catalog)
	return selected
}

type neutralFixtureCatalog struct {
	directory *ExecutionDirectory
	catalog   ProfileCatalog
}

var neutralFixtureCatalogs sync.Map

func rememberNeutralCatalog(config Config, catalog ProfileCatalog) {
	neutralFixtureCatalogs.Store(config.fixedProfileDigest, neutralFixtureCatalog{directory: catalog.Directory, catalog: catalog})
}

func neutralFixtureRuntime(config Config) (RuntimeDependencies, ProfileCatalog) {
	if value, ok := neutralFixtureCatalogs.Load(config.fixedProfileDigest); ok {
		fixture := value.(neutralFixtureCatalog)
		return RuntimeDependencies{Directory: fixture.directory, ProcessHost: testProcessHost{}, Profiles: neutralTestProfilePort{fixture}}, fixture.catalog
	}
	directory, err := newNeutralDirectory()
	if err != nil {
		panic(err)
	}
	profile := ExecutionProfile{directory: directory, ID: config.selectedProfileID, Name: "Fixture", Host: config.Host, Agent: config.Agent,
		PublicationPolicy: config.PublicationPolicy, Publication: config.publicationSettings(),
		AcceptedConfigVersion: config.acceptedConfigVersion,
		ImplementationConfigs: implementationInputsFromData(config.ImplementationData), ImplementationData: CloneImplementationData(config.ImplementationData), ImplementationDisplay: append([]string(nil), config.ImplementationDisplay...), projectIdentity: config.Identity()}
	profile.directory = directory
	catalog := ProfileCatalog{Base: config, Directory: directory, Profiles: []ExecutionProfile{profile}, DefaultID: profile.ID, Global: true}
	rememberNeutralCatalog(config, catalog)
	return RuntimeDependencies{Directory: directory, ProcessHost: testProcessHost{}, Profiles: neutralTestProfilePort{neutralFixtureCatalog{directory: directory, catalog: catalog}}}, catalog
}

func testProfileCatalogFor(config Config) ProfileCatalog {
	_, catalog := neutralFixtureRuntime(config)
	return catalog
}

func testProjectExecutionProfile(config Config) ExecutionProfile {
	dependencies, catalog := neutralFixtureRuntime(config)
	catalog.Base = config
	catalog.Directory = dependencies.Directory
	for _, profile := range catalog.AvailableExecutionProfiles() {
		if profile.ID == config.selectedProfileID {
			return profile
		}
	}
	return ExecutionProfile{}
}

func testTargetFor(config Config) executionbackend.ExecutionTarget {
	dependencies, _ := neutralFixtureRuntime(config)
	return TargetFor(config, dependencies.Directory)
}

func testRegistrationFor(config Config) (ExecutionRegistration, error) {
	dependencies, _ := neutralFixtureRuntime(config)
	return registrationFor(config, dependencies.Directory)
}

func testBackendFor(config Config) executionbackend.ExecutionBackend {
	dependencies, _ := neutralFixtureRuntime(config)
	return BackendFor(config, dependencies.Directory)
}

func testPublicationClient(config Config) *publication.Client {
	dependencies, _ := neutralFixtureRuntime(config)
	return publicationClient(config, dependencies.ProcessHost)
}

func testResolveProfile(config Config, id, digest string) (Config, executionmodel.FixedExecutionProfile, error) {
	dependencies, catalog := neutralFixtureRuntime(config)
	catalog.Base = config
	catalog.Directory = dependencies.Directory
	selected, profile, err := catalog.ResolveExecutionProfile(id, digest)
	if err != nil {
		return Config{}, executionmodel.FixedExecutionProfile{}, err
	}
	rememberNeutralCatalog(selected, catalog)
	return selected, profile.Freeze(dependencies.Directory, selected.Identity()), nil
}

type neutralTestProfilePort struct{ fixture neutralFixtureCatalog }

func (p neutralTestProfilePort) DefaultProfileID() string {
	return p.fixture.catalog.DefaultProfileID()
}
func (p neutralTestProfilePort) HasMultipleProfiles() bool {
	return p.fixture.catalog.HasMultipleProfiles()
}
func (p neutralTestProfilePort) ResolveExecutionProfile(id, digest string) (Config, executionmodel.FixedExecutionProfile, error) {
	config, profile, err := p.fixture.catalog.ResolveExecutionProfile(id, digest)
	if err != nil {
		return Config{}, executionmodel.FixedExecutionProfile{}, err
	}
	return config, profile.Freeze(p.fixture.directory, config.Identity()), nil
}

func setNeutralProfile(t *testing.T, c *Config, value neutralSettings) {
	t.Helper()
	*c = withNeutralProfile(*c, value)
}

func neutralBuildPolicyResource(_ context.Context, r PolicyResourceBuildRequest) (PolicyResourceBuildResult, error) {
	e, err := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{ImplementationKey: neutralPolicyKey, CodecVersion: 1, Purpose: executionbackend.ImplementationResourcePurpose, ProjectID: r.Resource.ProjectID, ExecutionID: r.Resource.ExecutionID, AttemptID: r.Resource.AttemptID, Generation: r.Resource.Generation, ConfigDigest: r.Resource.ConfigDigest}, mustJSON(struct {
		Workspace string `json:"workspace"`
	}{r.Workspace}))
	return PolicyResourceBuildResult{Envelope: e, WriteRoots: []string{r.Workspace}}, err
}

func neutralVerifyPolicyResource(e executionbackend.ImplementationDataEnvelope, id PolicyResourceIdentity) error {
	if e.ImplementationKey != neutralPolicyKey || e.CodecVersion != 1 {
		return errors.New("fixture policy codec differs")
	}
	return e.ValidateResourceIdentity(id.ProjectID, id.ExecutionID, id.AttemptID, id.Generation, id.ConfigDigest)
}

func setNeutralBinding(t *testing.T, e *executionmodel.ExecutionRecord, endpoint, generation, session string) {
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
	envelope, err := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{ImplementationKey: neutralBindingKey, CodecVersion: 1, Purpose: executionbackend.ImplementationResourcePurpose, ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest}, mustJSON(neutralBinding{endpoint, generation, session}))
	require(t, err)
	require(t, putImplementationEnvelope(e, envelope))
	e.SessionID, e.SessionGeneration = session, generation
}

func decodeNeutralBinding(envelope executionbackend.ImplementationDataEnvelope, record *executionmodel.ExecutionRecord) (neutralBinding, error) {
	var value neutralBinding
	err := envelope.ValidateResourceIdentity(record.ProjectIdentity, record.ExecutionID, record.CurrentAttemptID, record.Generation, record.ConfigurationDigest)
	if err == nil {
		err = decode(envelope.Payload, &value)
	}
	return value, err
}

func neutralDecisionFixture(t *testing.T, fixed executionbackend.SessionContext, id string, questions []executionbackend.Question) executionmodel.DecisionRequest {
	t.Helper()
	for i := range questions {
		questions[i].ExecutionID, questions[i].AttemptID, questions[i].Generation, questions[i].TurnID = fixed.ExecutionID, fixed.AttemptID, fixed.Generation, fixed.CurrentTurnID
	}
	route := simpleQuestionRoute(t, neutralQuestionKey, fixed.ProjectID, fixed.WorkflowRunID, fixed.ExecutionID, fixed.AttemptID, fixed.Generation, fixed.CurrentTurnID, id)
	return DecisionRequestFromEventFact(executionbackend.EventFact{QuestionRoute: &route, Questions: questions})
}

func neutralPolicyReceipt(profile, path string, e *executionmodel.ExecutionRecord) *executionmodel.PolicyResources {
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
	r := &executionmodel.PolicyResources{Version: 3, ProfileDigest: profile, Workspace: path, WriteRoots: []string{path}, TemporaryRoots: []string{}, ImplementationData: result.Envelope}
	r.Digest = PolicyResourceDigest(*r)
	return r
}

func newNeutralService(c Config) (*Service, error) {
	if c.selectedProfileID == "" {
		c = withNeutralProfile(c, neutralSettings{Executable: os.Args[0]})
	}
	dependencies, _ := neutralFixtureRuntime(c)
	var err error
	err = c.ValidateAdmission(dependencies.Directory)
	if err != nil {
		return nil, err
	}
	db, err := openFixtureCommandStore(context.Background(), &c)
	if err != nil {
		return nil, err
	}
	stores := ServiceStorage{State: db, Commands: db, Worker: db, Conversations: db, Records: db, Resources: fixtureResources{}, Close: db.Close}
	work := newFixtureTasks(db, c)
	service, err := NewService(c, dependencies, RuntimeSettings{Claimant: "operator", Heartbeat: 5 * time.Second, ShutdownGrace: 10 * time.Second}, dependencies.Directory.LifecycleResolver(), neutralConversationSources{}, stores, work, work, fixtureHost{config: c})
	if err != nil {
		_ = db.Close()
	}
	return service, err
}

func neutralVerifyFixedExecutionProfile(p executionmodel.FixedExecutionProfile) bool {
	d, err := newNeutralDirectory()
	return err == nil && VerifyFixedExecutionProfile(p, d)
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

func neutralProfileFromInputs(inputs []executionbackend.ImplementationConfigInput) neutralSettings {
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
