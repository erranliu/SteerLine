package steerlineintegration

import (
	"context"
	"errors"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
	"os"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"github.com/erranliu/SteerLine/internal/steerlineapp"
)

// Native fixtures encode configuration values and use the actual app catalog.
// They contain no copy of its implementation factories or center write state.
func builtinFixtureConfig(t *testing.T, c steerline.Config, worker codexworker.ProfileConfig, terminal *orcaworker.ProfileConfig, remoteSettings ...*remote.ProfileConfig) steerline.Config {
	t.Helper()
	host := steerlineapp.NewProcessHost(10 * time.Second)
	directory, err := steerlineapp.NewBuiltinDirectory(host)
	require(t, err)
	c.ProcessHost = host
	accepted := c.LoadedConfigDigest()
	if accepted == "" {
		accepted = "fixture-native-accepted-v1"
	}
	c.BindApplicationConfig(directory, "fixture-project.json", "fixture-global.json", accepted, true)
	p := steerline.ExecutionProfile{ID: steerline.ProjectExecutionProfileID, Name: "Native fixture", Host: c.Host, Agent: c.Agent}
	payload, err := codexworker.EncodeProfileConfig(worker)
	require(t, err)
	p.ImplementationConfigs = append(p.ImplementationConfigs, executionapi.ImplementationConfigInput{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 1, Payload: payload})
	if terminal != nil {
		payload, err := orcaworker.EncodeProfileConfig(*terminal)
		require(t, err)
		p.ImplementationConfigs = append(p.ImplementationConfigs, executionapi.ImplementationConfigInput{ImplementationKey: orcaworker.ProfileCodecKey, CodecVersion: 1, Payload: payload})
	}
	if len(remoteSettings) > 0 && remoteSettings[0] != nil {
		payload, err := remote.EncodeProfileConfig(*remoteSettings[0])
		require(t, err)
		p.ImplementationConfigs = append(p.ImplementationConfigs, executionapi.ImplementationConfigInput{ImplementationKey: remote.ProfileCodecKey, CodecVersion: 1, Payload: payload})
	}
	c.ExecutionProfiles = []steerline.ExecutionProfile{p}
	c.DefaultExecutionProfile = p.ID
	c = c.ApplyExecutionProfile(p)
	// A static rejection is still inspected as raw configuration input in
	// these negative fixtures. Preserve its scoped codec values for read-only
	// diagnostics; ValidateProfiles and Claim continue to reject admission.
	if len(c.ImplementationData) == 0 {
		project := c.Identity()
		if project == "" {
			project = "unresolved-project"
		}
		for _, input := range p.ImplementationConfigs {
			e, err := executionapi.NewImplementationDataEnvelope(executionapi.ImplementationDataEnvelope{ImplementationKey: input.ImplementationKey, CodecVersion: input.CodecVersion, Purpose: executionapi.ImplementationConfigPurpose, ProjectID: project, ProfileID: p.ID, AcceptedConfigVersion: accepted}, input.Payload)
			require(t, err)
			c.ImplementationData = append(c.ImplementationData, e)
		}
	}
	return c
}
func nativeProfileSettings(c steerline.Config) codexworker.ProfileConfig {
	data, _ := c.DecodeImplementationConfig(codexworker.ProfileCodecKey)
	if len(data) == 0 {
		for _, p := range c.ExecutionProfiles {
			for _, input := range p.ImplementationConfigs {
				if input.ImplementationKey == codexworker.ProfileCodecKey {
					data = input.Payload
				}
			}
		}
	}
	value, _ := codexworker.DecodeProfileConfig(data)
	return value
}
func fixtureBoundProfile(c steerline.Config) steerline.ExecutionProfile {
	for _, p := range c.AvailableExecutionProfiles() {
		if p.Host == c.Host && p.Agent == c.Agent {
			return p
		}
	}
	panic("fixture profile was not projected")
}
func fixtureOriginalResource(t *testing.T, e *steerline.ExecutionRecord, endpoint, generation, session string) {
	t.Helper()
	if e.ProjectIdentity == "" {
		e.ProjectIdentity = "fixture-project"
	}
	if e.ExecutionID == "" {
		e.ExecutionID = "fixture-execution"
	}
	if e.CurrentAttemptID == "" {
		e.CurrentAttemptID = "fixture-attempt"
	}
	if e.ConfigurationDigest == "" {
		e.ConfigurationDigest = "fixture-config"
	}
	if e.Generation == "" {
		e.Generation = "fixture-generation"
	}
	envelope, err := codexworker.NewAppServerBindingEnvelope(codexworker.AppServerBindingIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest}, codexworker.AppServerBinding{Endpoint: endpoint, Generation: generation, SessionID: session})
	require(t, err)
	e.ImplementationData = []executionapi.ImplementationDataEnvelope{envelope}
	e.SessionID, e.SessionGeneration = session, generation
}

func workspacePolicyFixture() executionapi.ExecutionPolicy {
	return executionapi.ExecutionPolicy{Filesystem: executionapi.FilesystemPolicy{ReadScope: "host", WriteScope: "workspace", GitMetadataWrite: true}, Network: "allow", Escalation: "deny"}
}
func executionPublicationFixture() executionapi.PublicationPolicy {
	return executionapi.PublicationPolicy{Publisher: executionapi.PublisherHostService}
}
func hostPublicationProviderFixture() publication.Config {
	return publication.Config{Kind: "command", BaseURL: "https://github.com", Repository: "owner/repo", Query: steerline.Command{Executable: os.Args[0], Args: []string{"api", "repos/{repository}/pulls/{number}"}}, Close: steerline.Command{Executable: os.Args[0], Args: []string{"pr", "close", "{pr_url}"}}}
}
func setNativeImplementationProfiles(t *testing.T, c *steerline.Config, w codexworker.ProfileConfig, o *orcaworker.ProfileConfig, r ...*remote.ProfileConfig) {
	*c = builtinFixtureConfig(t, *c, w, o, r...)
}
func setNativeProfile(t *testing.T, c *steerline.Config, w codexworker.ProfileConfig) {
	var o *orcaworker.ProfileConfig
	var r []*remote.ProfileConfig
	if c.Host != "local" {
		data, _ := c.DecodeImplementationConfig(orcaworker.ProfileCodecKey)
		v, _ := orcaworker.DecodeProfileConfig(data)
		o = &v
	}
	if c.Host == "orca_supervised" {
		data, _ := c.DecodeImplementationConfig(remote.ProfileCodecKey)
		v, _ := remote.DecodeProfileConfig(data)
		r = append(r, &v)
	}
	setNativeImplementationProfiles(t, c, w, o, r...)
}
func nativeAdmissionFixture(t *testing.T) (steerline.Config, string, *steerline.SQLiteStore) {
	t.Helper()
	f := newLifecycleProject(t, "local")
	store, err := steerline.OpenNativeTaskCommandStore(context.Background(), &f.config)
	require(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return f.config, f.commit, store
}
func nativeConfigError(c steerline.Config) error {
	if err := c.ValidateProfiles(); err != nil {
		return err
	}
	entry, err := c.ExecutionDirectory.Resolve(c)
	if err != nil {
		return err
	}
	return entry.ValidateConfig(context.Background(), c)
}
func historicalProfileValid(p steerline.FixedExecutionProfile) bool {
	d, err := steerlineapp.NewBuiltinDirectory(steerlineapp.NewProcessHost(time.Second))
	return err == nil && (steerline.Config{ExecutionDirectory: d}).CheckFixedProfile(p) == nil
}
func historicalResourceError(_ steerline.Config, r *contracts.PolicyResources, digest, path string, e *steerline.ExecutionRecord) error {
	if r == nil || r.Version != 3 {
		return errors.New("historical resource wrapper unsupported")
	}
	_, err := codexworker.DecodePolicyResourceEnvelope(r.ImplementationData, codexworker.PolicyResourceIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: digest})
	return err
}
func digestValue(value any) string { return string(mustJSON(value)) }

func nativeRequirementConfig(t *testing.T) steerline.Config {
	return builtinFixtureConfig(t, steerline.Config{Host: "local", Agent: "codex", WorkflowProtocol: steerline.WorkerFlowVersion, TaskAuthority: "native", ExecutionPolicy: workspacePolicyFixture(), PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture()}, codexworker.ProfileConfig{Executable: os.Args[0]}, nil)
}
func nativeTarget(c steerline.Config) executionapi.ExecutionTarget {
	entry, err := c.ExecutionDirectory.Resolve(c)
	if err != nil {
		return executionapi.ExecutionTarget{}
	}
	return entry.Target(c)
}
func requireNativeFlowCapabilities(ctx context.Context, c steerline.Config) error {
	if err := c.ValidateProfiles(); err != nil {
		return err
	}
	entry, err := c.ExecutionDirectory.Resolve(c)
	if err != nil {
		return err
	}
	if err := entry.ValidateConfig(ctx, c); err != nil {
		return err
	}
	return entry.Preflight(ctx, c)
}
