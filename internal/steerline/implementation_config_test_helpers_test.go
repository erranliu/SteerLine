package steerline

import (
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func testExecutionProfile(t *testing.T, id, name, host, agent string, codex codexworker.ProfileConfig, orca *orcaworker.ProfileConfig) ExecutionProfile {
	t.Helper()
	profile := ExecutionProfile{directory: testImplementationDirectory(t), ID: id, Name: name, Host: host, Agent: agent}
	codexPayload, err := codexworker.EncodeProfileConfig(codex)
	if err != nil {
		t.Fatal(err)
	}
	profile.ImplementationConfigs = append(profile.ImplementationConfigs, executionapi.ImplementationConfigInput{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 1, Payload: codexPayload})
	if host != "local" {
		if orca == nil {
			t.Fatal("non-local profile requires Orca codec input")
		}
		payload, err := orcaworker.EncodeProfileConfig(*orca)
		if err != nil {
			t.Fatal(err)
		}
		profile.ImplementationConfigs = append(profile.ImplementationConfigs, executionapi.ImplementationConfigInput{ImplementationKey: orcaworker.ProfileCodecKey, CodecVersion: 1, Payload: payload})
	}
	return profile
}

func setTestImplementationProfiles(t *testing.T, config *Config, codex codexworker.ProfileConfig, orca *orcaworker.ProfileConfig) {
	t.Helper()
	*config = withTestImplementationProfiles(*config, codex, orca)
}

func withTestImplementationProfiles(config Config, codex codexworker.ProfileConfig, orca *orcaworker.ProfileConfig) Config {
	config = testConfigWithDirectory(config)
	if config.configDigest == "" {
		config.configDigest = "test-accepted-config-v2"
	}
	if config.Host == "" {
		config.Host = "local"
	}
	if config.Agent == "" {
		config.Agent = "codex"
	}
	profileID := ProjectExecutionProfileID
	projectID := configurationProjectIdentity(config)
	codexPayload, err := codexworker.EncodeProfileConfig(codex)
	if err != nil {
		panic(err)
	}
	codexEnvelope, err := executionapi.NewImplementationDataEnvelope(executionapi.ImplementationDataEnvelope{
		CodecVersion: 1, Purpose: executionapi.ImplementationConfigPurpose,
		ImplementationKey: codexworker.ProfileCodecKey, ProjectID: projectID,
		ProfileID: profileID, AcceptedConfigVersion: config.configDigest,
	}, codexPayload)
	if err != nil {
		panic(err)
	}
	data := []executionapi.ImplementationDataEnvelope{codexEnvelope}
	if config.Host != "local" {
		if orca == nil {
			panic("non-local test profile requires Orca settings")
		}
		payload, err := orcaworker.EncodeProfileConfig(*orca)
		if err != nil {
			panic(err)
		}
		envelope, err := executionapi.NewImplementationDataEnvelope(executionapi.ImplementationDataEnvelope{
			CodecVersion: 1, Purpose: executionapi.ImplementationConfigPurpose,
			ImplementationKey: orcaworker.ProfileCodecKey, ProjectID: projectID,
			ProfileID: profileID, AcceptedConfigVersion: config.configDigest,
		}, payload)
		if err != nil {
			panic(err)
		}
		data = append(data, envelope)
	}
	config.ImplementationData = cloneImplementationData(data)
	config.ImplementationDisplay = nil
	config.selectedProfileID = profileID
	profile := config.projectExecutionProfile()
	config.ImplementationData = cloneImplementationData(profile.ImplementationData)
	config.ImplementationDisplay = append([]string(nil), profile.ImplementationDisplay...)
	config.fixedProfileDigest = profile.DigestFor(projectID)
	return config
}

func setTestCodexProfile(t *testing.T, config *Config, codex codexworker.ProfileConfig) {
	t.Helper()
	var orca *orcaworker.ProfileConfig
	if config.Host != "" && config.Host != "local" {
		settings := orcaProfileSettings(*config)
		orca = &settings
	}
	setTestImplementationProfiles(t, config, codex, orca)
}

func setTestCodexBinding(t *testing.T, record *ExecutionRecord, endpoint, generation, sessionID string) {
	t.Helper()
	if record.ProjectIdentity == "" {
		record.ProjectIdentity = "test-project"
	}
	if record.ExecutionID == "" {
		record.ExecutionID = "test-execution"
	}
	if record.CurrentAttemptID == "" {
		record.CurrentAttemptID = "test-attempt"
	}
	if record.ConfigurationDigest == "" {
		record.ConfigurationDigest = "test-config-digest"
	}
	if record.Generation == "" {
		record.Generation = "test-resource-generation"
	}
	envelope, err := codexworker.NewAppServerBindingEnvelope(codexworker.AppServerBindingIdentity{
		ProjectID: record.ProjectIdentity, ExecutionID: record.ExecutionID, AttemptID: record.CurrentAttemptID,
		Generation: record.Generation, ConfigDigest: record.ConfigurationDigest,
	}, codexworker.AppServerBinding{Endpoint: endpoint, Generation: generation, SessionID: sessionID})
	if err != nil {
		t.Fatal(err)
	}
	if record.Process != nil {
		process := *record.Process
		record.Process = &process
	}
	if err := putImplementationEnvelope(record, envelope); err != nil {
		t.Fatal(err)
	}
	record.SessionID, record.SessionGeneration = sessionID, generation
}

func testOrcaState(t *testing.T, record *ExecutionRecord) *implementationExecutionView {
	t.Helper()
	state, err := decodeOrcaExecutionState(record)
	if err != nil {
		t.Fatal(err)
	}
	return state
}
