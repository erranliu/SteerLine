package execution

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
)

const ProjectExecutionProfileID = "project-default"
const fixedExecutionProfileVersion = 6

// ExecutionProfile is one app catalog entry. It contains serialized profile
// inputs and derived values, but no directory, process host or profile catalog.
type ExecutionProfile struct {
	directory              *ExecutionDirectory
	ExecutionRequirements  executionbackend.ExecutionRequirements        `json:"-"`
	RequirementsEvaluation *executionbackend.RequirementsEvaluation      `json:"-"`
	PublicationPolicy      executionbackend.PublicationPolicy            `json:"-"`
	Publication            executionmodel.PublicationSettings            `json:"-"`
	ExecutionPolicy        executionbackend.ExecutionPolicy              `json:"-"`
	ImplementationConfigs  []executionbackend.ImplementationConfigInput  `json:"implementation_configs"`
	ImplementationData     []executionbackend.ImplementationDataEnvelope `json:"-"`
	ImplementationDisplay  []string                                      `json:"-"`
	AcceptedConfigVersion  string                                        `json:"-"`
	PolicySource           string                                        `json:"-"`
	ID                     string                                        `json:"id"`
	Name                   string                                        `json:"name"`
	Host                   string                                        `json:"host"`
	Agent                  string                                        `json:"agent"`
	projectIdentity        string
}

// ProfileCatalog is a short-lived app-side projection used to validate and
// select one entry. It is never retained by an execution service.
type ProfileCatalog struct {
	Base         Config
	Directory    *ExecutionDirectory
	Profiles     []ExecutionProfile
	DefaultID    string
	Global       bool
	PolicySource string
}

func (catalog ProfileCatalog) CheckFixedProfile(profile executionmodel.FixedExecutionProfile) error {
	if !VerifyFixedExecutionProfile(profile, catalog.Directory) {
		return &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "fixed execution profile key, version, codec or digest is unsupported"}
	}
	return nil
}

func (p ExecutionProfile) Freeze(args ...any) executionmodel.FixedExecutionProfile {
	directory := p.directory
	var projectID string
	for _, arg := range args {
		switch value := arg.(type) {
		case *ExecutionDirectory:
			directory = value
		case string:
			projectID = value
		}
	}
	if directory == nil {
		return executionmodel.FixedExecutionProfile{}
	}
	if projectID == "" {
		projectID = p.projectIdentity
	}
	if projectID == "" {
		projectID = "unresolved-project"
	}
	accepted := p.AcceptedConfigVersion
	if accepted == "" {
		accepted = "unaccepted-profile-input"
	}
	data, display, _ := directory.NormalizeProfile(p.ImplementationConfigs, p.ExecutionPolicy, p.Host, p.Agent, projectID, p.ID, accepted)
	p.ExecutionRequirements = p.ExecutionRequirements.Clone()
	if p.RequirementsEvaluation != nil {
		evaluation := p.RequirementsEvaluation.Clone()
		p.RequirementsEvaluation = &evaluation
	}
	p.Publication = clonePublicationSettings(p.Publication)
	fixed := executionmodel.FixedExecutionProfile{
		Version: fixedExecutionProfileVersion, ImplementationVersion: directory.Version(),
		ImplementationKey: profileImplementationKey(directory, p.Host, p.Agent), ImplementationData: CloneImplementationData(data),
		ImplementationDisplay: slices.Clone(display), ExecutionRequirements: p.ExecutionRequirements,
		RequirementsEvaluation: p.RequirementsEvaluation, RequirementsEvidenceDigest: requirementsEvidenceDigest(p.RequirementsEvaluation),
		PublicationPolicy: p.PublicationPolicy, Publication: p.Publication, ExecutionPolicy: p.ExecutionPolicy,
		PolicyDigest: digestValue(p.ExecutionPolicy), AcceptedConfigVersion: accepted, PolicySource: p.PolicySource,
		ID: p.ID, Name: p.Name, Host: p.Host, Agent: p.Agent,
	}
	fixed.Digest = fixedExecutionProfileDigest(fixed)
	return fixed
}

func fixedExecutionProfileDigest(p executionmodel.FixedExecutionProfile) string {
	p.Digest = ""
	return digestValue(p)
}

func cloneRequirementsEvaluation(value *executionbackend.RequirementsEvaluation) *executionbackend.RequirementsEvaluation {
	if value == nil {
		return nil
	}
	copy := value.Clone()
	return &copy
}

func VerifyFixedExecutionProfile(p executionmodel.FixedExecutionProfile, directory *ExecutionDirectory) bool {
	if directory == nil || p.Version != fixedExecutionProfileVersion || p.ImplementationVersion != directory.Version() ||
		p.ImplementationKey == "" || p.ID == "" || p.Name == "" || p.AcceptedConfigVersion == "" ||
		p.PolicySource == "" || p.Digest == "" || p.Digest != fixedExecutionProfileDigest(p) ||
		!verifyRequirementsSnapshot(p, directory) ||
		publication.ValidateSettings(p.Publication) != nil || p.ExecutionPolicy.Validate() != nil ||
		p.PolicyDigest != digestValue(p.ExecutionPolicy) {
		return false
	}
	entry, err := directory.Resolve(Config{Host: p.Host, Agent: p.Agent})
	if err != nil || entry.ValidatePublicationPolicy(p.PublicationPolicy, publication.Config(p.Publication.Provider)) != nil {
		return false
	}
	if len(p.ImplementationData) == 0 {
		return false
	}
	projectID := p.ImplementationData[0].ProjectID
	inputs := make([]executionbackend.ImplementationConfigInput, 0, len(p.ImplementationData))
	for _, envelope := range p.ImplementationData {
		if err := envelope.ValidateProfileIdentity(projectID, p.ID, p.AcceptedConfigVersion); err != nil {
			return false
		}
		inputs = append(inputs, executionbackend.ImplementationConfigInput{ImplementationKey: envelope.ImplementationKey, CodecVersion: envelope.CodecVersion, Payload: append([]byte(nil), envelope.Payload...)})
	}
	data, display, err := directory.NormalizeProfile(inputs, p.ExecutionPolicy, p.Host, p.Agent, projectID, p.ID, p.AcceptedConfigVersion)
	if err != nil || digestValue(data) != digestValue(p.ImplementationData) || digestValue(display) != digestValue(p.ImplementationDisplay) {
		return false
	}
	return p.ImplementationKey == profileImplementationKey(directory, p.Host, p.Agent)
}

func (c Config) ApplyFixedExecutionProfile(p executionmodel.FixedExecutionProfile, directory *ExecutionDirectory) Config {
	c.frozenExecutionConfig = true
	c.ExecutionRequirements = p.ExecutionRequirements.Clone()
	c.requirementsEvaluation = cloneRequirementsEvaluation(p.RequirementsEvaluation)
	c.PublicationPolicy = p.PublicationPolicy
	c.PR, c.Git, c.Remote, c.publicationRemoteURL, c.Branch = p.Publication.Provider, p.Publication.Git, p.Publication.Remote, p.Publication.RemoteURL, p.Publication.BaseBranch
	c.Host, c.Agent = p.Host, p.Agent
	if directory != nil {
		if entry, err := directory.Resolve(Config{Host: p.Host, Agent: p.Agent}); err == nil {
			c.WorkspaceBranchPrefix = entry.WorkspaceBranchPrefix
		}
	}
	c.ImplementationData = CloneImplementationData(p.ImplementationData)
	c.ImplementationDisplay = slices.Clone(p.ImplementationDisplay)
	c.selectedProfileID = p.ID
	c.fixedProfileDigest = p.Digest
	c.acceptedConfigVersion = p.AcceptedConfigVersion
	c.ExecutionPolicy = p.ExecutionPolicy
	return copyWorkerFlowConfig(c)
}

func (p ExecutionProfile) Digest(args ...any) string {
	return p.DigestFor(args...)
}

func (p ExecutionProfile) DigestFor(args ...any) string {
	directory := p.directory
	projectID := p.projectIdentity
	for _, arg := range args {
		switch value := arg.(type) {
		case *ExecutionDirectory:
			directory = value
		case string:
			projectID = value
		}
	}
	if directory == nil {
		return ""
	}
	if projectID == "" {
		projectID = "unresolved-project"
	}
	accepted := p.AcceptedConfigVersion
	if accepted == "" {
		accepted = "unaccepted-profile-input"
	}
	if _, _, err := directory.NormalizeProfile(p.ImplementationConfigs, p.ExecutionPolicy, p.Host, p.Agent, projectID, p.ID, accepted); err != nil {
		return ""
	}
	return p.Freeze(directory, projectID).Digest
}

func (p ExecutionProfile) StaticUnavailableReason(directories ...*ExecutionDirectory) string {
	directory := p.directory
	if len(directories) > 0 {
		directory = directories[0]
	}
	if directory == nil {
		return "execution directory is required"
	}
	if err := p.ExecutionRequirements.Validate(); err != nil {
		return err.Error()
	}
	entry, err := directory.Resolve(Config{Host: p.Host, Agent: p.Agent})
	if err != nil {
		return err.Error()
	}
	if err := entry.ValidatePublicationPolicy(p.PublicationPolicy, publication.Config(p.Publication.Provider)); err != nil {
		return err.Error()
	}
	if err := publication.ValidateSettings(p.Publication); err != nil {
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
	if _, _, err := directory.NormalizeProfile(p.ImplementationConfigs, p.ExecutionPolicy, p.Host, p.Agent, projectID, p.ID, accepted); err != nil {
		return err.Error()
	}
	return ""
}

func (c ProfileCatalog) bindProjectPolicy(p ExecutionProfile) ExecutionProfile {
	p.directory = c.Directory
	p.ImplementationConfigs = cloneImplementationInputs(p.ImplementationConfigs)
	p.ExecutionRequirements = c.Base.ExecutionRequirements.Clone()
	p.RequirementsEvaluation = cloneRequirementsEvaluation(c.Base.requirementsEvaluation)
	p.PublicationPolicy = c.Base.PublicationPolicy
	p.Publication = clonePublicationSettings(c.Base.publicationSettings())
	p.ExecutionPolicy, p.AcceptedConfigVersion, p.PolicySource = c.Base.ExecutionPolicy, c.Base.acceptedConfigVersion, c.PolicySource
	if p.AcceptedConfigVersion == "" {
		p.AcceptedConfigVersion = "unaccepted-profile-input"
	}
	if p.PolicySource == "" {
		p.PolicySource = "service:explicit-config"
	}
	p.projectIdentity = ConfigurationProjectIdentity(c.Base)
	if c.Directory != nil {
		data, display, err := c.Directory.NormalizeProfileInputs(p.ImplementationConfigs, p.ExecutionPolicy, p.Host, p.Agent, p.projectIdentity, p.ID, p.AcceptedConfigVersion)
		if err == nil {
			p.ImplementationData, p.ImplementationDisplay = data, display
		}
	}
	return p
}

func (c ProfileCatalog) projectExecutionProfile() ExecutionProfile {
	inputs := implementationInputsFromData(c.Base.ImplementationData)
	p := ExecutionProfile{ID: ProjectExecutionProfileID, Name: "项目默认", Host: c.Base.hostName(), Agent: c.Base.agentName(), ImplementationConfigs: inputs}
	return c.bindProjectPolicy(p)
}

func (c ProfileCatalog) AvailableExecutionProfiles() []ExecutionProfile {
	if c.Global {
		out := make([]ExecutionProfile, 0, len(c.Profiles))
		for _, profile := range c.Profiles {
			out = append(out, c.bindProjectPolicy(profile))
		}
		return out
	}
	out := make([]ExecutionProfile, 0, len(c.Profiles)+1)
	out = append(out, c.projectExecutionProfile())
	for _, profile := range c.Profiles {
		out = append(out, c.bindProjectPolicy(profile))
	}
	return out
}

func (c ProfileCatalog) DefaultProfileID() string {
	if c.DefaultID != "" {
		return c.DefaultID
	}
	if c.Global {
		if len(c.Profiles) == 1 {
			return c.Profiles[0].ID
		}
		return ""
	}
	if len(c.Profiles) == 0 {
		return ProjectExecutionProfileID
	}
	return ""
}

func (c ProfileCatalog) HasMultipleProfiles() bool { return len(c.AvailableExecutionProfiles()) > 1 }

func (c ProfileCatalog) ResolveExecutionProfile(id, expectedDigest string) (Config, ExecutionProfile, error) {
	for _, profile := range c.AvailableExecutionProfiles() {
		if profile.ID != id {
			continue
		}
		if expectedDigest != "" && expectedDigest != profile.DigestFor(c.Directory, ConfigurationProjectIdentity(c.Base)) {
			return Config{}, ExecutionProfile{}, errors.New("stale_profile: execution profile changed")
		}
		if reason := profile.StaticUnavailableReason(c.Directory); reason != "" {
			return Config{}, ExecutionProfile{}, errors.New(reason)
		}
		return c.SelectExecutionProfile(profile.ID)
	}
	return Config{}, ExecutionProfile{}, errors.New("execution profile is not configured")
}

// SelectExecutionProfile projects one app-owned catalog entry without deciding
// whether its policy is currently available. Admission remains a separate step.
func (c ProfileCatalog) SelectExecutionProfile(id string) (Config, ExecutionProfile, error) {
	for _, profile := range c.AvailableExecutionProfiles() {
		if profile.ID != id {
			continue
		}
		selected := c.Base
		selected.PublicationPolicy = profile.PublicationPolicy
		selected.ExecutionRequirements = profile.ExecutionRequirements.Clone()
		selected.requirementsEvaluation = cloneRequirementsEvaluation(profile.RequirementsEvaluation)
		selected.ExecutionPolicy = profile.ExecutionPolicy
		selected.PR, selected.Git = profile.Publication.Provider, profile.Publication.Git
		selected.Remote, selected.Branch, selected.publicationRemoteURL = profile.Publication.Remote, profile.Publication.BaseBranch, profile.Publication.RemoteURL
		selected.Host, selected.Agent = profile.Host, profile.Agent
		selected.ImplementationData = CloneImplementationData(profile.ImplementationData)
		selected.ImplementationDisplay = slices.Clone(profile.ImplementationDisplay)
		selected.selectedProfileID = profile.ID
		selected.fixedProfileDigest = profile.DigestFor(c.Directory, ConfigurationProjectIdentity(c.Base))
		selected.acceptedConfigVersion = profile.AcceptedConfigVersion
		if c.Directory != nil {
			if entry, err := c.Directory.Resolve(Config{Host: profile.Host, Agent: profile.Agent}); err == nil {
				selected.WorkspaceBranchPrefix = entry.WorkspaceBranchPrefix
			}
		}
		return copyWorkerFlowConfig(selected), profile, nil
	}
	return Config{}, ExecutionProfile{}, errors.New("execution profile is not configured")
}

func (c ProfileCatalog) Validate() error {
	if c.Directory == nil {
		return errors.New("execution directory is required")
	}
	if err := c.Base.ExecutionRequirements.Validate(); err != nil {
		return err
	}
	if err := c.Base.PublicationPolicy.Validate(); err != nil {
		return err
	}
	if err := publication.ValidateNonSecret(publication.Config(c.Base.PR), c.Base.Git); err != nil {
		return err
	}
	if err := c.Base.ExecutionPolicy.Validate(); err != nil {
		return err
	}
	ids := map[string]bool{}
	if !c.Global {
		ids[ProjectExecutionProfileID] = true
	}
	pattern := regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	for _, profile := range c.Profiles {
		if !pattern.MatchString(profile.ID) || ids[profile.ID] || strings.TrimSpace(profile.Name) == "" {
			return fmt.Errorf("execution profile ID or name is invalid: %q", profile.ID)
		}
		ids[profile.ID] = true
		bound := c.bindProjectPolicy(profile)
		if reason := bound.StaticUnavailableReason(c.Directory); reason != "" {
			return fmt.Errorf("execution profile %s: %s", profile.ID, reason)
		}
	}
	if c.DefaultID != "" && !ids[c.DefaultID] {
		return errors.New("default_execution_profile is not configured")
	}
	return nil
}

func profileImplementationKey(directory *ExecutionDirectory, host, agent string) string {
	if directory == nil {
		return ""
	}
	entry, err := directory.Resolve(Config{Host: host, Agent: agent})
	if err != nil {
		return ""
	}
	return entry.ProfileKey
}

func CloneImplementationData(values []executionbackend.ImplementationDataEnvelope) []executionbackend.ImplementationDataEnvelope {
	if values == nil {
		return nil
	}
	out := make([]executionbackend.ImplementationDataEnvelope, len(values))
	for i := range values {
		out[i] = values[i].Clone()
	}
	return out
}

func cloneImplementationInputs(values []executionbackend.ImplementationConfigInput) []executionbackend.ImplementationConfigInput {
	if values == nil {
		return nil
	}
	out := make([]executionbackend.ImplementationConfigInput, len(values))
	for i, value := range values {
		out[i] = value
		out[i].Payload = append([]byte(nil), value.Payload...)
	}
	return out
}

func ConfigurationProjectIdentity(c Config) string {
	if c.identity != "" {
		return c.identity
	}
	if c.Repository != "" {
		return canonical(c.Repository)
	}
	return "unresolved-project"
}
