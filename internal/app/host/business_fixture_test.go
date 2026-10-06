package host

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	execution "github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

const neutralProfileKey = "fixture.config"

const neutralBindingKey = "fixture.binding"

const neutralQuestionKey = "fixture.question"

const neutralPolicyKey = "fixture.policy"

const neutralProtocol executionbackend.ExecutionProtocol = "fixture_stream_v1"

type testExecutionResources struct{}

func (testExecutionResources) WorkerTreeAbsent(identity executionbackend.ProcessIdentity) bool {
	return resourceprocess.WorkerTreeAbsent(identity)
}

func (testExecutionResources) AtomicWrite(path string, data []byte) error {
	return AtomicWrite(path, data)
}

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

func executionInputsForHostFixture(c execution.Config) []executionbackend.ImplementationConfigInput {
	inputs := make([]executionbackend.ImplementationConfigInput, 0, len(c.ImplementationData))
	for _, value := range c.ImplementationData {
		inputs = append(inputs, executionbackend.ImplementationConfigInput{ImplementationKey: value.ImplementationKey, CodecVersion: value.CodecVersion, Payload: append([]byte(nil), value.Payload...)})
	}
	return inputs
}

func neutralCodecs() []execution.ImplementationProfileCodec {
	return []execution.ImplementationProfileCodec{{Key: neutralProfileKey, Version: 1, Normalize: func(data []byte, policy executionbackend.ExecutionPolicy, host, agent string) ([]byte, []string, error) {
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

func newNeutralDirectory(agents ...string) (*execution.ExecutionDirectory, error) {
	var directory *execution.ExecutionDirectory
	key := execution.ExecutionRegistrationKey{Backend: executionbackend.BackendLocal, Protocol: neutralProtocol, Agent: "fixture"}
	entry := execution.ExecutionRegistration{Key: key, ProfileKey: "fixture/stream-v1", Host: "local", WorkspaceBranchPrefix: "fixture", LocalWorkspace: true, ProfileCodecKeys: []string{neutralProfileKey}, QuestionCodecKey: neutralQuestionKey, QuestionCodecVersion: 1,
		Target: func(c execution.Config) executionbackend.ExecutionTarget {
			target := neutralProfileSettings(c).Target
			if target.ID == "" {
				target.ID = "fixture:fixed"
			}
			return target
		},
		Backend: func(c execution.Config) executionbackend.ExecutionBackend { return neutralBackend{config: c} },
		Lifecycle: func(setup executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
			return &neutralLifecycle{setup: setup.Clone()}, nil
		},
		ProbeRequirements: func(_ context.Context, c execution.Config) (executionbackend.CapabilityReport, error) {
			return capabilityReport(key.Backend, key.Protocol, execution.TargetFor(c, directory), execution.AgentFor(c), "fixture-version-1"), nil
		},
		QueryTool:      queryHostRequirementTool,
		ValidateConfig: func(context.Context, execution.Config) error { return nil }, Preflight: func(context.Context, execution.Config) error { return nil }, PrepareClaim: func(context.Context, execution.Config) (execution.ClaimPreparation, error) {
			return execution.ClaimPreparation{}, nil
		}, ReadinessRequired: func(execution.Config) bool { return false },
		BuildPolicyResource: neutralBuildPolicyResource, VerifyPolicyResource: neutralVerifyPolicyResource,
		LinkedWorktreeWriteArgs: func(context.Context, execution.Config, string) ([]string, error) { return nil, nil }, ValidTurnID: func(value string) bool { return value != "" }, CleanupCapabilities: []string{"start", "observe", "stop", "workspace_inspection", "output_reading"},
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
	entries := make([]execution.ExecutionRegistration, 0, len(agents))
	for _, agent := range agents {
		registration := entry
		registration.Key.Agent = agent
		if agent != "fixture" {
			registration.ProfileKey = "fixture/stream-v1/" + agent
		}
		entries = append(entries, registration)
	}
	var err error
	directory, err = execution.NewExecutionDirectory("fixture-directory-v1", neutralCodecs(), entries)
	return directory, err
}

func neutralProfileSettings(c execution.Config) neutralSettings {
	data, err := c.DecodeImplementationConfig(neutralProfileKey)
	if err != nil {
		return neutralSettings{}
	}
	var value neutralSettings
	_ = decode(data, &value)
	return value
}

func neutralProfile(t *testing.T, id, name string, value neutralSettings) execution.ExecutionProfile {
	t.Helper()
	return execution.ExecutionProfile{ID: id, Name: name, Host: "local", Agent: "fixture",
		ImplementationConfigs: []executionbackend.ImplementationConfigInput{{ImplementationKey: neutralProfileKey, CodecVersion: 1, Payload: mustJSON(value)}}}
}

type hostFixtureCatalog struct {
	directory *execution.ExecutionDirectory
	catalog   execution.ProfileCatalog
}

var hostFixtureCatalogs sync.Map

func rememberHostFixtureCatalog(c execution.Config, catalog execution.ProfileCatalog) {
	hostFixtureCatalogs.Store(c.FixedProfileDigest(), hostFixtureCatalog{directory: catalog.Directory, catalog: catalog})
}

func hostFixtureRuntime(c execution.Config) execution.RuntimeDependencies {
	if value, ok := hostFixtureCatalogs.Load(c.FixedProfileDigest()); ok {
		fixture := value.(hostFixtureCatalog)
		return execution.RuntimeDependencies{Directory: fixture.directory, ProcessHost: testProcessHost{}, Profiles: hostTestProfilePort{fixture}}
	}
	directory, err := newNeutralDirectory()
	if err != nil {
		panic(err)
	}
	profile := execution.ExecutionProfile{ID: execution.ProjectExecutionProfileID, Name: "Fixture", Host: c.Host, Agent: c.Agent,
		ExecutionPolicy: c.ExecutionPolicy, PublicationPolicy: c.PublicationPolicy, ImplementationConfigs: executionInputsForHostFixture(c)}
	catalog := execution.ProfileCatalog{Base: c, Directory: directory, Profiles: []execution.ExecutionProfile{profile}, DefaultID: profile.ID, Global: true, PolicySource: "fixture:config"}
	fixture := hostFixtureCatalog{directory: directory, catalog: catalog}
	rememberHostFixtureCatalog(c, catalog)
	return execution.RuntimeDependencies{Directory: directory, ProcessHost: testProcessHost{}, Profiles: hostTestProfilePort{fixture}}
}

func hostFixtureCatalogFor(c execution.Config) execution.ProfileCatalog {
	if value, ok := hostFixtureCatalogs.Load(c.FixedProfileDigest()); ok {
		return value.(hostFixtureCatalog).catalog
	}
	return hostFixtureRuntime(c).Profiles.(hostTestProfilePort).fixture.catalog
}

type hostTestProfilePort struct{ fixture hostFixtureCatalog }

func (p hostTestProfilePort) DefaultProfileID() string { return p.fixture.catalog.DefaultProfileID() }
func (p hostTestProfilePort) HasMultipleProfiles() bool {
	return p.fixture.catalog.HasMultipleProfiles()
}
func (p hostTestProfilePort) ResolveExecutionProfile(id, digest string) (execution.Config, executionmodel.FixedExecutionProfile, error) {
	config, profile, err := p.fixture.catalog.ResolveExecutionProfile(id, digest)
	if err != nil {
		return execution.Config{}, executionmodel.FixedExecutionProfile{}, err
	}
	return config, profile.Freeze(p.fixture.directory, config.Identity()), nil
}
func withNeutralProfile(c execution.Config, value neutralSettings) execution.Config {
	_ = resolveHostTestIdentity(context.Background(), &c)
	directory, err := newNeutralDirectory()
	if err != nil {
		panic(err)
	}
	digest := c.AcceptedConfigVersion()
	if digest == "" {
		digest = "fixture-accepted-v1"
	}
	c.BindAcceptedConfigVersion(digest)
	c.BindRepositoryIdentity(c.Identity())
	if c.WorktreeRoot == "" && c.RunDir != "" {
		c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	}
	p := execution.ExecutionProfile{ID: execution.ProjectExecutionProfileID, Name: "项目默认", Host: "local", Agent: "fixture",
		ImplementationConfigs: []executionbackend.ImplementationConfigInput{{ImplementationKey: neutralProfileKey, CodecVersion: 1, Payload: mustJSON(value)}}}
	catalog := execution.ProfileCatalog{Base: c, Directory: directory, Profiles: []execution.ExecutionProfile{p}, DefaultID: p.ID, Global: true, PolicySource: "fixture:config"}
	selected, _, err := catalog.ResolveExecutionProfile(p.ID, "")
	if err != nil {
		panic(err)
	}
	rememberHostFixtureCatalog(selected, catalog)
	return selected
}

func setNeutralProfile(t *testing.T, c *execution.Config, value neutralSettings) {
	t.Helper()
	*c = withNeutralProfile(*c, value)
}

func neutralBuildPolicyResource(_ context.Context, r execution.PolicyResourceBuildRequest) (execution.PolicyResourceBuildResult, error) {
	e, err := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{ImplementationKey: neutralPolicyKey, CodecVersion: 1, Purpose: executionbackend.ImplementationResourcePurpose, ProjectID: r.Resource.ProjectID, ExecutionID: r.Resource.ExecutionID, AttemptID: r.Resource.AttemptID, Generation: r.Resource.Generation, ConfigDigest: r.Resource.ConfigDigest}, mustJSON(struct {
		Workspace string `json:"workspace"`
	}{r.Workspace}))
	return execution.PolicyResourceBuildResult{Envelope: e, WriteRoots: []string{r.Workspace}}, err
}

func neutralVerifyPolicyResource(e executionbackend.ImplementationDataEnvelope, id execution.PolicyResourceIdentity) error {
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
	require(t, putFixtureImplementationEnvelope(e, envelope))
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
	return execution.DecisionRequestFromEventFact(executionbackend.EventFact{QuestionRoute: &route, Questions: questions})
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
	result, err := neutralBuildPolicyResource(context.Background(), execution.PolicyResourceBuildRequest{Workspace: path, Resource: execution.PolicyResourceIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: profile}})
	if err != nil {
		panic(err)
	}
	r := &executionmodel.PolicyResources{Version: 3, ProfileDigest: profile, Workspace: path, WriteRoots: []string{path}, TemporaryRoots: []string{}, ImplementationData: result.Envelope}
	r.Digest = execution.PolicyResourceDigest(*r)
	return r
}

func newNeutralService(c execution.Config) (*execution.Service, error) {
	dependencies := hostFixtureRuntime(c)
	var err error
	c, err = testPrepareServiceConfig(c)
	if err != nil {
		return nil, err
	}
	db, err := openHostFixtureNativeCommandStore(context.Background(), &c)
	if err != nil {
		return nil, err
	}
	stores := execution.ServiceStorage{State: db, Commands: db, Worker: db, Conversations: db, Records: db, Resources: testExecutionResources{}, Close: db.Close}
	work := fullFixtureTaskService(db, c)
	service, err := execution.NewService(c, dependencies, execution.RuntimeSettings{Claimant: hostConfigForTest(c).Claimant, Heartbeat: 5 * time.Second, ShutdownGrace: 10 * time.Second}, dependencies.Directory.LifecycleResolver(), neutralConversationSources{}, stores, work, work, NewServiceHost(hostConfigForTest(c), "", "", nil, testHostAdmission{config: c}, nil))
	if err != nil {
		_ = db.Close()
	}
	return service, err
}

func readFixtureDatabaseRuntime(ctx context.Context, c execution.Config) (*reconstruction.Runtime, error) {
	reader, err := openHostFixtureNativeReader(ctx, &c)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	state, err := reader.ReadExecutionStateSnapshot(ctx, c.Identity())
	if err != nil {
		return nil, err
	}
	if state.Checkpoint == nil {
		return nil, storageports.ErrNotFound
	}
	runtimeState, err := reconstruction.BuildFromStorage(state, *state.Checkpoint)
	if err != nil {
		return nil, err
	}
	return &runtimeState, nil
}

func neutralVerifyFixedExecutionProfile(p executionmodel.FixedExecutionProfile) bool {
	d, err := newNeutralDirectory()
	return err == nil && execution.VerifyFixedExecutionProfile(p, d)
}
