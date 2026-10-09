package execution

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

// ExecutionRegistrationKey is the persisted, generic identity of one execution
// implementation. Its values are supplied by the application directory.
type ExecutionRegistrationKey struct {
	Backend  executionbackend.BackendKind
	Agent    string
	Protocol executionbackend.ExecutionProtocol
}

// ImplementationProfileCodec owns the wire format for one profile payload.
// The center only envelopes bytes and asks this supplied codec to normalize
// them; it never interprets implementation fields.
type ImplementationProfileCodec struct {
	Key       string
	Version   int
	Normalize func([]byte, string, string) ([]byte, []string, error)
}

// ClaimPreparation carries only center-owned timing and gate inputs. The
// implementation validates its fixed launch configuration before a claim;
// nonce, deadline, authorization and persistence stay in the center.
type ClaimPreparation struct {
	RequiresReadiness bool
	WaitTimeout       time.Duration
}

type PolicyResourceIdentity struct {
	ProjectID    string
	ExecutionID  string
	AttemptID    string
	Generation   string
	ConfigDigest string
}

type PolicyResourceBuildRequest struct {
	ProcessHost           executionbackend.ManagedProcessHost `json:"-"`
	ProfileData           []executionbackend.ImplementationDataEnvelope
	ProjectID             string
	ProfileID             string
	AcceptedConfigVersion string
	Resource              PolicyResourceIdentity

	Host               string
	Agent              string
	Workspace          string
	LinkedWorktreeArgs []string
	RepositoryPath     string
	CommandTimeout     time.Duration
}

type PolicyResourceBuildResult struct {
	Envelope   executionbackend.ImplementationDataEnvelope
	WriteRoots []string
}

// ExecutionRegistration binds generic service ports to one fixed execution
// identity. The application supplies all factories and platform behavior.
type ExecutionRegistration struct {
	Key                   ExecutionRegistrationKey
	ProfileKey            string
	WorkspaceBranchPrefix string
	LocalWorkspace        bool
	ProfileCodecKeys      []string
	QuestionCodecKey      string
	QuestionCodecVersion  int
	Host                  string
	Backend               func(Config) executionbackend.ExecutionBackend
	Target                func(Config) executionbackend.ExecutionTarget

	Lifecycle                 func(executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error)
	ValidateProfileSelection  func(string, map[string][]byte) error
	ValidateConfig            func(context.Context, Config) error
	Preflight                 func(context.Context, Config) error
	PrepareClaim              func(context.Context, Config) (ClaimPreparation, error)
	ReadinessRequired         func(Config) bool
	BuildPolicyResource       func(context.Context, PolicyResourceBuildRequest) (PolicyResourceBuildResult, error)
	VerifyPolicyResource      func(executionbackend.ImplementationDataEnvelope, PolicyResourceIdentity) error
	LinkedWorktreeWriteArgs   func(context.Context, Config, string) ([]string, error)
	ValidTurnID               func(string) bool
	CleanupCapabilities       []string
	ValidateRef               func(executionbackend.ExecutionRef) error
	ValidatePublicationPolicy func(executionbackend.PublicationPolicy, publication.Config) error
}

func (directory *ExecutionDirectory) ValidateRef(ref executionbackend.ExecutionRef) error {
	if err := executionbackend.ValidateExecutionRef(ref); err != nil {
		return err
	}
	entry, err := directory.ResolveIdentity(ExecutionRegistrationKey{Backend: ref.Backend, Protocol: ref.Protocol, Agent: ref.Agent.Name})
	if err != nil {
		return err
	}
	if entry.ValidateRef == nil {
		return errors.New("execution implementation has no resource identity validator")
	}
	return entry.ValidateRef(ref)
}

// ExecutionDirectory is immutable after construction and belongs to one
// application instance. It contains no process-wide registration state.
type ExecutionDirectory struct {
	version string
	entries map[ExecutionRegistrationKey]ExecutionRegistration
	routes  map[string]ExecutionRegistrationKey
	codecs  map[string]ImplementationProfileCodec
}

func NewExecutionDirectory(version string, codecs []ImplementationProfileCodec, entries []ExecutionRegistration) (*ExecutionDirectory, error) {
	if strings.TrimSpace(version) == "" {
		return nil, errors.New("execution directory version is required")
	}
	directory := &ExecutionDirectory{
		version: version,
		entries: make(map[ExecutionRegistrationKey]ExecutionRegistration, len(entries)),
		routes:  make(map[string]ExecutionRegistrationKey, len(entries)),
		codecs:  make(map[string]ImplementationProfileCodec, len(codecs)),
	}
	for _, codec := range codecs {
		if codec.Key == "" || codec.Version <= 0 || codec.Normalize == nil {
			return nil, errors.New("execution directory contains an incomplete profile codec")
		}
		if _, exists := directory.codecs[codec.Key]; exists {
			return nil, fmt.Errorf("duplicate profile codec key %q", codec.Key)
		}
		directory.codecs[codec.Key] = codec
	}
	profileKeys := map[string]bool{}
	for _, entry := range entries {
		key := entry.Key
		if profileKeys[entry.ProfileKey] {
			return nil, fmt.Errorf("duplicate implementation profile key %q", entry.ProfileKey)
		}
		profileKeys[entry.ProfileKey] = true
		if key.Backend == "" || key.Agent == "" || key.Protocol == "" || entry.Host == "" || entry.ProfileKey == "" || entry.WorkspaceBranchPrefix == "" || entry.ValidTurnID == nil || len(entry.CleanupCapabilities) == 0 {
			return nil, fmt.Errorf("execution directory contains an incomplete identity: %+v", key)
		}
		if _, exists := directory.entries[key]; exists {
			return nil, fmt.Errorf("duplicate execution identity: %+v", key)
		}
		route := directoryRoute(entry.Host, key.Agent)
		if _, exists := directory.routes[route]; exists {
			return nil, fmt.Errorf("ambiguous execution route %q", route)
		}
		if entry.Lifecycle == nil || entry.QuestionCodecKey == "" || entry.QuestionCodecVersion <= 0 || entry.Backend == nil || entry.Target == nil || entry.ValidateConfig == nil || entry.Preflight == nil || entry.PrepareClaim == nil || entry.ReadinessRequired == nil || entry.BuildPolicyResource == nil || entry.VerifyPolicyResource == nil || entry.LinkedWorktreeWriteArgs == nil || entry.ValidatePublicationPolicy == nil || entry.ValidateRef == nil {
			return nil, fmt.Errorf("execution directory contains incomplete operations: %+v", key)
		}
		if len(entry.ProfileCodecKeys) == 0 {
			return nil, fmt.Errorf("execution directory identity has no profile codecs: %+v", key)
		}
		seenCodecs := map[string]bool{}
		for _, codecKey := range entry.ProfileCodecKeys {
			if _, exists := directory.codecs[codecKey]; !exists || seenCodecs[codecKey] {
				return nil, fmt.Errorf("execution directory identity has an unknown or duplicate profile codec %q", codecKey)
			}
			seenCodecs[codecKey] = true
		}
		entry.ProfileCodecKeys = slices.Clone(entry.ProfileCodecKeys)
		entry.CleanupCapabilities = slices.Clone(entry.CleanupCapabilities)
		directory.entries[key] = entry
		directory.routes[route] = key
	}
	if len(directory.entries) == 0 || len(directory.codecs) == 0 {
		return nil, errors.New("execution directory requires profile codecs and implementations")
	}
	return directory, nil
}

func directoryRoute(host, agent string) string { return host + "\x00" + agent }

// A host-owned opaque agent route delegates agent interpretation to that host.
// Exact registrations retain priority and returned identities bind the actual name.
func (directory *ExecutionDirectory) route(host, agent string) (ExecutionRegistrationKey, bool) {
	if strings.TrimSpace(agent) == "" || agent == "*" {
		return ExecutionRegistrationKey{}, false
	}
	key, ok := directory.routes[directoryRoute(host, agent)]
	if !ok {
		key, ok = directory.routes[directoryRoute(host, "*")]
	}
	return key, ok
}

func (directory *ExecutionDirectory) Version() string {
	if directory == nil {
		return ""
	}
	return directory.version
}

func (directory *ExecutionDirectory) Resolve(c Config) (ExecutionRegistration, error) {
	if directory == nil {
		return ExecutionRegistration{}, errors.New("execution directory is required")
	}
	key, ok := directory.route(c.hostName(), c.agentName())
	if !ok {
		return ExecutionRegistration{}, &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: fmt.Sprintf("no implementation is configured for host=%s agent=%s", c.hostName(), c.agentName())}
	}
	entry := directory.entries[key]
	entry.Key.Agent = c.agentName()
	entry.ProfileCodecKeys = slices.Clone(entry.ProfileCodecKeys)
	entry.CleanupCapabilities = slices.Clone(entry.CleanupCapabilities)
	return entry, nil
}

func (directory *ExecutionDirectory) ResolveIdentity(key ExecutionRegistrationKey) (ExecutionRegistration, error) {
	if directory == nil {
		return ExecutionRegistration{}, errors.New("execution directory is required")
	}
	entry, ok := directory.entries[key]
	if !ok && strings.TrimSpace(key.Agent) != "" && key.Agent != "*" {
		wildcard := key
		wildcard.Agent = "*"
		entry, ok = directory.entries[wildcard]
	}
	if !ok {
		return ExecutionRegistration{}, &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: "no implementation is registered for the fixed execution identity"}
	}
	entry.Key.Agent = key.Agent
	entry.ProfileCodecKeys = slices.Clone(entry.ProfileCodecKeys)
	entry.CleanupCapabilities = slices.Clone(entry.CleanupCapabilities)
	return entry, nil
}

func (directory *ExecutionDirectory) NormalizeProfile(inputs []executionbackend.ImplementationConfigInput, host, agent, projectID, profileID, acceptedVersion string) ([]executionbackend.ImplementationDataEnvelope, []string, error) {
	return directory.normalizeProfile(inputs, host, agent, projectID, profileID, acceptedVersion, true)
}

// NormalizeProfileInputs canonicalizes app-owned profile inputs into frozen
// envelopes without deciding whether the selected target is currently
// supported. The app uses this projection for query and diagnostics paths;
// admission still calls NormalizeProfile, which also validates selection.
func (directory *ExecutionDirectory) NormalizeProfileInputs(inputs []executionbackend.ImplementationConfigInput, host, agent, projectID, profileID, acceptedVersion string) ([]executionbackend.ImplementationDataEnvelope, []string, error) {
	return directory.normalizeProfile(inputs, host, agent, projectID, profileID, acceptedVersion, false)
}

func (directory *ExecutionDirectory) normalizeProfile(inputs []executionbackend.ImplementationConfigInput, host, agent, projectID, profileID, acceptedVersion string, validateSelection bool) ([]executionbackend.ImplementationDataEnvelope, []string, error) {
	if directory == nil {
		return nil, nil, errors.New("execution directory is required")
	}
	if projectID == "" || profileID == "" || acceptedVersion == "" {
		return nil, nil, errors.New("implementation configuration is missing project, profile or accepted version")
	}
	entryKey, ok := directory.route(host, agent)
	if !ok {
		return nil, nil, &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: fmt.Sprintf("no implementation is configured for host=%s agent=%s", host, agent)}
	}
	entry := directory.entries[entryKey]
	inputs = cloneImplementationInputs(inputs)
	slices.SortFunc(inputs, func(a, b executionbackend.ImplementationConfigInput) int {
		return strings.Compare(a.ImplementationKey, b.ImplementationKey)
	})
	seen := map[string]bool{}
	payloads := make(map[string][]byte, len(inputs))
	data := make([]executionbackend.ImplementationDataEnvelope, 0, len(inputs))
	display := []string{}
	for _, input := range inputs {
		codec, exists := directory.codecs[input.ImplementationKey]
		if !exists || input.CodecVersion != codec.Version || seen[input.ImplementationKey] {
			return nil, nil, fmt.Errorf("unknown, duplicate or unsupported implementation codec %q/%d", input.ImplementationKey, input.CodecVersion)
		}
		seen[input.ImplementationKey] = true
		payload, projection, err := codec.Normalize(input.Payload, host, agent)
		if err != nil {
			return nil, nil, fmt.Errorf("implementation codec %s rejected profile: %w", codec.Key, err)
		}
		envelope, err := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{
			CodecVersion: codec.Version, Purpose: executionbackend.ImplementationConfigPurpose,
			ImplementationKey: codec.Key, ProjectID: projectID, ProfileID: profileID,
			AcceptedConfigVersion: acceptedVersion,
		}, payload)
		if err != nil {
			return nil, nil, err
		}
		data = append(data, envelope)
		payloads[codec.Key] = append([]byte(nil), envelope.Payload...)
		display = append(display, projection...)
	}
	if len(data) != len(entry.ProfileCodecKeys) {
		return nil, nil, errors.New("execution profile has incomplete implementation configuration")
	}
	for _, required := range entry.ProfileCodecKeys {
		if !seen[required] {
			return nil, nil, fmt.Errorf("execution profile is missing required codec %q", required)
		}
	}
	if validateSelection && entry.ValidateProfileSelection != nil {
		if err := entry.ValidateProfileSelection(agent, payloads); err != nil {
			return nil, nil, err
		}
	}
	return data, display, nil
}

func (directory *ExecutionDirectory) DecodeProfile(data []executionbackend.ImplementationDataEnvelope, key, projectID, profileID, acceptedVersion string) ([]byte, error) {
	if directory == nil {
		return nil, errors.New("execution directory is required")
	}
	var found *executionbackend.ImplementationDataEnvelope
	for i := range data {
		if data[i].ImplementationKey != key {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("duplicate implementation profile envelope %q", key)
		}
		found = &data[i]
	}
	if found == nil {
		return nil, fmt.Errorf("missing implementation profile envelope %q", key)
	}
	if err := found.ValidateProfileIdentity(projectID, profileID, acceptedVersion); err != nil {
		return nil, err
	}
	codec, ok := directory.codecs[key]
	if !ok || found.CodecVersion != codec.Version {
		return nil, fmt.Errorf("unsupported implementation profile codec %q/%d", key, found.CodecVersion)
	}
	return append([]byte(nil), found.Payload...), nil
}

func (directory *ExecutionDirectory) ValidateProfileData(c Config) error {
	if c.selectedProfileID == "" {
		return errors.New("configuration has no selected implementation profile")
	}
	inputs := implementationInputsFromData(c.ImplementationData)
	data, _, err := directory.NormalizeProfile(inputs, c.hostName(), c.agentName(), ConfigurationProjectIdentity(c), c.selectedProfileID, c.acceptedConfigVersion)
	if err != nil {
		return err
	}
	if !equalImplementationData(data, c.ImplementationData) {
		return errors.New("implementation profile payload is not canonical or its identity changed")
	}
	return nil
}

func (directory *ExecutionDirectory) LifecycleResolver() executionbackend.LifecycleResolver {
	return directory
}

func (directory *ExecutionDirectory) ResolveLifecycle(setup executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
	entry, err := directory.ResolveIdentity(ExecutionRegistrationKey{Backend: setup.Snapshot.Ref.Backend, Agent: setup.Snapshot.Ref.Agent.Name, Protocol: setup.Snapshot.Ref.Protocol})
	if err != nil {
		return nil, err
	}
	return entry.Lifecycle(setup.Clone())
}

func equalImplementationData(a, b []executionbackend.ImplementationDataEnvelope) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ImplementationKey != b[i].ImplementationKey || a[i].CodecVersion != b[i].CodecVersion || a[i].Purpose != b[i].Purpose || a[i].ProjectID != b[i].ProjectID || a[i].ProfileID != b[i].ProfileID || a[i].AcceptedConfigVersion != b[i].AcceptedConfigVersion || a[i].Digest != b[i].Digest || string(a[i].Payload) != string(b[i].Payload) {
			return false
		}
	}
	return true
}

func registrationFor(c Config, directory *ExecutionDirectory) (ExecutionRegistration, error) {
	if directory == nil {
		return ExecutionRegistration{}, errors.New("execution directory is required")
	}
	return directory.Resolve(c)
}

func workerFlowRegistrationFor(config Config, directory *ExecutionDirectory, run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord) (ExecutionRegistration, error) {
	if run == nil || execution == nil {
		return ExecutionRegistration{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "worker flow has no fixed run or execution"}
	}
	entry, err := registrationFor(config, directory)
	if err != nil {
		return ExecutionRegistration{}, err
	}
	if run.Host != entry.Host || run.Agent != entry.Key.Agent || execution.Agent != run.Agent || run.Backend != string(entry.Key.Backend) || execution.Backend != run.Backend || run.Protocol != string(entry.Key.Protocol) || execution.Protocol != run.Protocol || run.Target != execution.Target || !sameConfiguredExecutionTarget(run.Target, entry.Target(config)) {
		return ExecutionRegistration{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "fixed worker target differs from its persisted execution"}
	}
	return entry, nil
}

func sameConfiguredExecutionTarget(actual, configured executionbackend.ExecutionTarget) bool {
	return actual.ID != "" && actual.ID == configured.ID && actual.ConnectionID == configured.ConnectionID && actual.ExternalRuntime == configured.ExternalRuntime
}

func executionUsesLocalWorkspace(c Config, directory *ExecutionDirectory, backend executionbackend.BackendKind, protocol executionbackend.ExecutionProtocol, agent string) bool {
	if directory == nil {
		return false
	}
	entry, err := directory.ResolveIdentity(ExecutionRegistrationKey{Backend: backend, Protocol: protocol, Agent: agent})
	return err == nil && entry.LocalWorkspace
}

type rejectedExecutionBackend struct{ err error }

func (b rejectedExecutionBackend) Identity() (executionbackend.BackendKind, executionbackend.ExecutionProtocol) {
	return "", ""
}

func (b rejectedExecutionBackend) Probe(context.Context, executionbackend.ExecutionTarget, executionbackend.AgentSpec) (executionbackend.CapabilityReport, error) {
	return executionbackend.CapabilityReport{}, b.err
}

func (b rejectedExecutionBackend) Observe(context.Context, executionbackend.ExecutionRef) (executionbackend.Observation, error) {
	return executionbackend.Observation{}, b.err
}

func (b rejectedExecutionBackend) InspectWorkspace(context.Context, executionbackend.ExecutionRef) (executionbackend.WorkspaceInspection, error) {
	return executionbackend.WorkspaceInspection{}, b.err
}

func BackendFor(c Config, directory *ExecutionDirectory) executionbackend.ExecutionBackend {
	entry, err := registrationFor(c, directory)
	if err != nil {
		return rejectedExecutionBackend{err}
	}
	return entry.Backend(c)
}

func AgentFor(c Config) executionbackend.AgentSpec {
	return executionbackend.AgentSpec{Name: c.agentName()}
}

func TargetFor(c Config, directory *ExecutionDirectory) executionbackend.ExecutionTarget {
	entry, err := registrationFor(c, directory)
	if err != nil {
		return executionbackend.ExecutionTarget{}
	}
	return entry.Target(c)
}

func (s *Service) workerFlowRegistration(run *reconstructiontypes.RunView) (ExecutionRegistration, error) {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return ExecutionRegistration{}, err
	}
	if fixed.workflow.WorkflowVersion != executionmodel.Version {
		return ExecutionRegistration{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "unsupported fixed workflow version"}
	}
	return workerFlowRegistrationFor(fixed.config, s.runtime.Directory, run, fixed.execution)
}

func linkedWorktreeWriteArgs(ctx context.Context, c Config, directory *ExecutionDirectory, worktree string) ([]string, error) {
	entry, err := registrationFor(c, directory)
	if err != nil {
		return nil, err
	}
	if entry.LinkedWorktreeWriteArgs == nil {
		return nil, nil
	}
	return entry.LinkedWorktreeWriteArgs(ctx, c, worktree)
}
