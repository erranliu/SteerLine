package steerline

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
)

const ProjectExecutionProfileID = "project-default"
const PublisherExecutionProfile = "execution_profile"
const fixedExecutionProfileVersion = 5

// ExecutionProfile contains general routing/policy facts and opaque inputs for
// strict implementation codecs. Native profile fields live in their owners.
type ExecutionProfile struct {
	directory                   *ExecutionDirectory
	ConfiguredPublicationPolicy *executionapi.PublicationPolicy           `json:"publication_policy,omitempty"`
	ExecutionRequirements       executionapi.ExecutionRequirements        `json:"-"`
	RequirementsEvaluation      *executionapi.RequirementsEvaluation      `json:"-"`
	PublicationPolicy           executionapi.PublicationPolicy            `json:"-"`
	Publication                 contracts.PublicationSettings             `json:"-"`
	ExecutionPolicy             executionapi.ExecutionPolicy              `json:"-"`
	ImplementationConfigs       []executionapi.ImplementationConfigInput  `json:"implementation_configs"`
	ImplementationData          []executionapi.ImplementationDataEnvelope `json:"-"`
	ImplementationDisplay       []string                                  `json:"-"`
	AcceptedConfigVersion       string                                    `json:"-"`
	PolicySource                string                                    `json:"-"`
	ID                          string                                    `json:"id"`
	Name                        string                                    `json:"name"`
	Host                        string                                    `json:"host"`
	Agent                       string                                    `json:"agent"`
	projectIdentity             string
}

type FixedExecutionProfile = contracts.FixedExecutionProfile

// CheckFixedProfile reports whether the injected version directory can read
// this exact frozen profile without replacing its implementation identity.
func (c Config) CheckFixedProfile(profile FixedExecutionProfile) error {
	if !verifyFixedExecutionProfile(profile, c.ExecutionDirectory) {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "fixed execution profile key, version, codec or digest is unsupported"}
	}
	return nil
}

func (p ExecutionProfile) Freeze(projectIDs ...string) FixedExecutionProfile {
	if p.directory == nil {
		return FixedExecutionProfile{}
	}
	projectID := p.projectIdentity
	if len(projectIDs) > 0 && projectIDs[0] != "" {
		projectID = projectIDs[0]
	}
	if projectID == "" {
		projectID = "unresolved-project"
	}
	accepted := p.AcceptedConfigVersion
	if accepted == "" {
		accepted = "unaccepted-profile-input"
	}
	var data []executionapi.ImplementationDataEnvelope
	var display []string
	if p.directory != nil {
		data, display, _ = p.directory.NormalizeProfile(p.ImplementationConfigs, p.ExecutionPolicy, p.Host, p.Agent, projectID, p.ID, accepted)
	}
	p.ExecutionRequirements = p.ExecutionRequirements.Clone()
	if p.RequirementsEvaluation != nil {
		evaluation := p.RequirementsEvaluation.Clone()
		p.RequirementsEvaluation = &evaluation
	}
	p.Publication.Provider.Query.Args = slices.Clone(p.Publication.Provider.Query.Args)
	p.Publication.Provider.Close.Args = slices.Clone(p.Publication.Provider.Close.Args)
	p.Publication.Git.Args = slices.Clone(p.Publication.Git.Args)
	p.Publication.Create.Args = slices.Clone(p.Publication.Create.Args)
	p.Publication.AuthenticationRefs = slices.Clone(p.Publication.AuthenticationRefs)
	fixed := FixedExecutionProfile{
		Version: fixedExecutionProfileVersion, ImplementationVersion: p.directory.Version(),
		ImplementationKey: profileImplementationKey(p.directory, p.Host, p.Agent), ImplementationData: cloneImplementationData(data),
		ImplementationDisplay: slices.Clone(display), ExecutionRequirements: p.ExecutionRequirements,
		RequirementsEvaluation: p.RequirementsEvaluation, RequirementsEvidenceDigest: requirementsEvidenceDigest(p.RequirementsEvaluation),
		PublicationPolicy: p.PublicationPolicy, Publication: p.Publication, ExecutionPolicy: p.ExecutionPolicy,
		PolicyDigest: digestValue(p.ExecutionPolicy), AcceptedConfigVersion: accepted, PolicySource: p.PolicySource,
		ID: p.ID, Name: p.Name, Host: p.Host, Agent: p.Agent,
	}
	fixed.Digest = fixedExecutionProfileDigest(fixed)
	return fixed
}

func fixedExecutionProfileDigest(p FixedExecutionProfile) string {
	p.Digest = ""
	return digestValue(p)
}

func verifyFixedExecutionProfile(p FixedExecutionProfile, directory *ExecutionDirectory) bool {
	if directory == nil || p.Version != fixedExecutionProfileVersion || p.ImplementationVersion != directory.Version() ||
		p.ImplementationKey == "" || p.ID == "" || p.Name == "" || p.AcceptedConfigVersion == "" ||
		p.PolicySource == "" || p.Digest == "" || p.Digest != fixedExecutionProfileDigest(p) ||
		!verifyRequirementsSnapshot(p, directory) ||
		publication.ValidateSettings(p.Publication, p.PublicationPolicy) != nil || p.ExecutionPolicy.Validate() != nil ||
		p.PolicyDigest != digestValue(p.ExecutionPolicy) {
		return false
	}
	entry, err := directory.Resolve(Config{ExecutionDirectory: directory, Host: p.Host, Agent: p.Agent})
	if err != nil || entry.ValidatePublicationPolicy(p.PublicationPolicy, publication.Config(p.Publication.Provider)) != nil {
		return false
	}
	if p.PublicationPolicy.Publisher == PublisherExecutionProfile {
		return false // a frozen profile stores the selected worker/host publisher.
	}
	if len(p.ImplementationData) == 0 {
		return false
	}
	projectID := p.ImplementationData[0].ProjectID
	inputs := make([]executionapi.ImplementationConfigInput, 0, len(p.ImplementationData))
	for _, envelope := range p.ImplementationData {
		if err := envelope.ValidateProfileIdentity(projectID, p.ID, p.AcceptedConfigVersion); err != nil {
			return false
		}
		inputs = append(inputs, executionapi.ImplementationConfigInput{ImplementationKey: envelope.ImplementationKey, CodecVersion: envelope.CodecVersion, Payload: append([]byte(nil), envelope.Payload...)})
	}
	data, display, err := directory.NormalizeProfile(inputs, p.ExecutionPolicy, p.Host, p.Agent, projectID, p.ID, p.AcceptedConfigVersion)
	if err != nil || digestValue(data) != digestValue(p.ImplementationData) || digestValue(display) != digestValue(p.ImplementationDisplay) {
		return false
	}
	return p.ImplementationKey == profileImplementationKey(directory, p.Host, p.Agent)
}

func (c Config) ApplyFixedExecutionProfile(p FixedExecutionProfile) Config {
	c.frozenExecutionConfig = true
	c.ExecutionRequirements = p.ExecutionRequirements.Clone()
	c.requirementsEvaluation = p.RequirementsEvaluation
	c.profilePublication = false
	c.PublicationPolicy = p.PublicationPolicy
	c.PR, c.Git, c.Remote, c.publicationRemoteURL, c.Branch = publication.Config(p.Publication.Provider), p.Publication.Git, p.Publication.Remote, p.Publication.RemoteURL, p.Publication.BaseBranch
	c.Host, c.Agent = p.Host, p.Agent
	if c.ExecutionDirectory != nil {
		if entry, err := c.ExecutionDirectory.Resolve(Config{ExecutionDirectory: c.ExecutionDirectory, Host: p.Host, Agent: p.Agent}); err == nil {
			c.WorkspaceBranchPrefix = entry.WorkspaceBranchPrefix
		}
	}
	c.ImplementationData = cloneImplementationData(p.ImplementationData)
	c.ImplementationDisplay = slices.Clone(p.ImplementationDisplay)
	c.selectedProfileID = p.ID
	c.fixedProfileDigest = p.Digest
	c.ExecutionPolicy, c.configDigest, c.policySource = p.ExecutionPolicy, p.AcceptedConfigVersion, p.PolicySource
	return copyWorkerFlowConfig(c)
}

func (p ExecutionProfile) Digest() string { return p.DigestFor(p.projectIdentity) }

func (p ExecutionProfile) DigestFor(projectID string) string {
	if p.directory == nil {
		return ""
	}
	if projectID == "" {
		projectID = "unresolved-project"
	}
	accepted := p.AcceptedConfigVersion
	if accepted == "" {
		accepted = "unaccepted-profile-input"
	}
	if _, _, err := p.directory.NormalizeProfile(p.ImplementationConfigs, p.ExecutionPolicy, p.Host, p.Agent, projectID, p.ID, accepted); err != nil {
		return ""
	}
	// Selection and claim bind the same immutable envelope.
	return p.Freeze(projectID).Digest
}

func cloneRequirementsEvaluation(value *executionapi.RequirementsEvaluation) *executionapi.RequirementsEvaluation {
	if value == nil {
		return nil
	}
	copy := value.Clone()
	return &copy
}

func (p ExecutionProfile) MatchesConfig(c Config) bool {
	actual := c.projectExecutionProfile()
	actual.ID, actual.Name = p.ID, p.Name
	return p.ID != "" && actual.DigestFor(configurationProjectIdentity(c)) == p.DigestFor(configurationProjectIdentity(c))
}

func (p ExecutionProfile) StaticUnavailableReason() string {
	if p.directory == nil {
		return "execution directory is required"
	}
	if err := p.ExecutionRequirements.Validate(); err != nil {
		return err.Error()
	}
	entry, err := p.directory.Resolve(Config{ExecutionDirectory: p.directory, Host: p.Host, Agent: p.Agent})
	if err != nil {
		return err.Error()
	}
	if err := entry.ValidatePublicationPolicy(p.PublicationPolicy, publication.Config(p.Publication.Provider)); err != nil {
		return err.Error()
	}
	if err := publication.ValidateSettings(p.Publication, p.PublicationPolicy); err != nil {
		return err.Error()
	}
	projectID := p.projectIdentity
	if projectID == "" {
		projectID = "unresolved-project"
	}
	accepted := p.AcceptedConfigVersion
	if accepted == "" {
		accepted = "unaccepted-profile-input"
	}
	if _, _, err := p.directory.NormalizeProfile(p.ImplementationConfigs, p.ExecutionPolicy, p.Host, p.Agent, projectID, p.ID, accepted); err != nil {
		return err.Error()
	}
	return ""
}

func (c Config) bindProjectPolicy(p ExecutionProfile) ExecutionProfile {
	p.directory = c.ExecutionDirectory
	if p.ConfiguredPublicationPolicy != nil {
		policy := *p.ConfiguredPublicationPolicy
		p.ConfiguredPublicationPolicy = &policy
	}
	p.ImplementationConfigs = cloneImplementationInputs(p.ImplementationConfigs)
	p.ExecutionRequirements = c.ExecutionRequirements.Clone()
	p.RequirementsEvaluation = cloneRequirementsEvaluation(c.requirementsEvaluation)
	p.PublicationPolicy = c.PublicationPolicy
	if c.publicationUsesProfile() {
		p.PublicationPolicy = executionapi.PublicationPolicy{}
		if p.ConfiguredPublicationPolicy != nil {
			p.PublicationPolicy = *p.ConfiguredPublicationPolicy
		}
	}
	publicationConfig := c
	publicationConfig.PublicationPolicy = p.PublicationPolicy
	p.Publication = clonePublicationSettings(publicationConfig.publicationSettings())
	p.ExecutionPolicy, p.AcceptedConfigVersion, p.PolicySource = c.ExecutionPolicy, c.acceptedConfigVersion(), c.policySource
	if p.PolicySource == "" {
		p.PolicySource = "service:explicit-config"
	}
	p.projectIdentity = configurationProjectIdentity(c)
	if p.directory == nil {
		return p
	}
	data, display, err := p.directory.NormalizeProfile(p.ImplementationConfigs, p.ExecutionPolicy, p.Host, p.Agent, p.projectIdentity, p.ID, p.AcceptedConfigVersion)
	if err == nil {
		p.ImplementationData, p.ImplementationDisplay = data, display
	}
	return p
}

func (c Config) publicationUsesProfile() bool {
	return c.profilePublication || c.PublicationPolicy.Publisher == PublisherExecutionProfile
}

func validateProjectPublicationPolicy(p executionapi.PublicationPolicy) error {
	if p.Publisher == PublisherExecutionProfile {
		return nil
	}
	return p.Validate()
}

func (c Config) acceptedConfigVersion() string {
	if c.configDigest != "" {
		return c.configDigest
	}
	return digestValue(struct {
		ExecutionRequirements executionapi.ExecutionRequirements
		PublicationPolicy     executionapi.PublicationPolicy
		Publication           contracts.PublicationSettings
		Policy                executionapi.ExecutionPolicy
		Repository, Branch    string
	}{c.ExecutionRequirements, c.PublicationPolicy, c.publicationSettings(), c.ExecutionPolicy, c.Repository, c.Branch})
}

func (c Config) projectExecutionProfile() ExecutionProfile {
	inputs := implementationInputsFromData(c.ImplementationData)
	p := ExecutionProfile{ID: ProjectExecutionProfileID, Name: "项目默认", Host: c.hostName(), Agent: c.agentName(), ImplementationConfigs: inputs}
	if c.publicationUsesProfile() && c.PublicationPolicy.Publisher != PublisherExecutionProfile {
		policy := c.PublicationPolicy
		p.ConfiguredPublicationPolicy = &policy
	}
	return c.bindProjectPolicy(p)
}

func (c Config) AvailableExecutionProfiles() []ExecutionProfile {
	if c.globalExecution {
		out := make([]ExecutionProfile, 0, len(c.ExecutionProfiles))
		for _, profile := range c.ExecutionProfiles {
			out = append(out, c.bindProjectPolicy(profile))
		}
		return out
	}
	out := make([]ExecutionProfile, 0, len(c.ExecutionProfiles)+1)
	out = append(out, c.projectExecutionProfile())
	for _, profile := range c.ExecutionProfiles {
		out = append(out, c.bindProjectPolicy(profile))
	}
	return out
}

func (c Config) DefaultProfileID() string {
	if c.DefaultExecutionProfile != "" {
		return c.DefaultExecutionProfile
	}
	if c.globalExecution {
		if len(c.ExecutionProfiles) == 1 {
			return c.ExecutionProfiles[0].ID
		}
		return ""
	}
	if len(c.ExecutionProfiles) == 0 {
		return ProjectExecutionProfileID
	}
	return ""
}

func (c Config) ResolveExecutionProfile(id, expectedDigest string) (Config, ExecutionProfile, error) {
	for _, profile := range c.AvailableExecutionProfiles() {
		if profile.ID != id {
			continue
		}
		if expectedDigest != "" && expectedDigest != profile.DigestFor(configurationProjectIdentity(c)) {
			return Config{}, ExecutionProfile{}, errors.New("stale_profile: execution profile changed")
		}
		if reason := profile.StaticUnavailableReason(); reason != "" {
			return Config{}, ExecutionProfile{}, errors.New(reason)
		}
		selected := c.ApplyExecutionProfile(profile)
		return selected, profile, nil
	}
	return Config{}, ExecutionProfile{}, errors.New("execution profile is not configured")
}

func (c Config) ApplyExecutionProfile(p ExecutionProfile) Config {
	c.frozenExecutionConfig = false
	p = c.bindProjectPolicy(p)
	c.profilePublication = c.publicationUsesProfile()
	c.PublicationPolicy = p.PublicationPolicy
	c.Host, c.Agent = p.Host, p.Agent
	if c.ExecutionDirectory != nil {
		if entry, err := c.ExecutionDirectory.Resolve(Config{ExecutionDirectory: c.ExecutionDirectory, Host: p.Host, Agent: p.Agent}); err == nil {
			c.WorkspaceBranchPrefix = entry.WorkspaceBranchPrefix
		} else {
			c.WorkspaceBranchPrefix = ""
		}
	}
	c.ImplementationData = cloneImplementationData(p.ImplementationData)
	c.ImplementationDisplay = slices.Clone(p.ImplementationDisplay)
	c.selectedProfileID = p.ID
	c.fixedProfileDigest = p.DigestFor(configurationProjectIdentity(c))
	return copyWorkerFlowConfig(c)
}

func validateExecutionProfiles(c Config) error {
	if c.ExecutionDirectory == nil {
		return errors.New("execution directory is required")
	}
	if err := c.ExecutionRequirements.Validate(); err != nil {
		return err
	}
	if err := validateProjectPublicationPolicy(c.PublicationPolicy); err != nil {
		return err
	}
	if err := publication.ValidateNonSecret(c.PR, c.Git); err != nil {
		return err
	}
	if err := c.ExecutionPolicy.Validate(); err != nil {
		return err
	}
	ids := map[string]bool{}
	if !c.globalExecution {
		ids[ProjectExecutionProfileID] = true
	}
	pattern := regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	for _, profile := range c.ExecutionProfiles {
		if c.publicationUsesProfile() && profile.ConfiguredPublicationPolicy == nil {
			return fmt.Errorf("execution profile %s requires publication_policy when the project publisher is execution_profile", profile.ID)
		}
		if profile.ConfiguredPublicationPolicy != nil {
			if err := profile.ConfiguredPublicationPolicy.Validate(); err != nil {
				return fmt.Errorf("execution profile %s: %w", profile.ID, err)
			}
		}
		if !pattern.MatchString(profile.ID) || ids[profile.ID] || strings.TrimSpace(profile.Name) == "" {
			return fmt.Errorf("execution profile ID or name is invalid: %q", profile.ID)
		}
		ids[profile.ID] = true
		bound := c.bindProjectPolicy(profile)
		if reason := bound.StaticUnavailableReason(); reason != "" {
			return fmt.Errorf("execution profile %s: %s", profile.ID, reason)
		}
	}
	if c.DefaultExecutionProfile != "" && !ids[c.DefaultExecutionProfile] {
		return errors.New("default_execution_profile is not configured")
	}
	return nil
}

func profileImplementationKey(directory *ExecutionDirectory, host, agent string) string {
	if directory == nil {
		return ""
	}
	entry, err := directory.Resolve(Config{ExecutionDirectory: directory, Host: host, Agent: agent})
	if err != nil {
		return ""
	}
	return entry.ProfileKey
}

func cloneImplementationData(values []executionapi.ImplementationDataEnvelope) []executionapi.ImplementationDataEnvelope {
	if values == nil {
		return nil
	}
	out := make([]executionapi.ImplementationDataEnvelope, len(values))
	for i := range values {
		out[i] = values[i].Clone()
	}
	return out
}

func cloneImplementationInputs(values []executionapi.ImplementationConfigInput) []executionapi.ImplementationConfigInput {
	if values == nil {
		return nil
	}
	out := make([]executionapi.ImplementationConfigInput, len(values))
	for i, value := range values {
		out[i] = value
		out[i].Payload = append([]byte(nil), value.Payload...)
	}
	return out
}

func configurationProjectIdentity(c Config) string {
	if c.identity != "" {
		return c.identity
	}
	if c.Repository != "" {
		return canonical(c.Repository)
	}
	return "unresolved-project"
}
