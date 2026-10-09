package orca_remote_worker

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

const ProfileCodecKey = "orca.supervised-profile"

func NormalizeProfilePayload(payload []byte) ([]byte, []string, error) {
	settings, err := DecodeProfileConfig(payload)
	if err != nil {
		return nil, nil, err
	}
	encoded, err := EncodeProfileConfig(settings)
	return encoded, []string{"目标环境：" + settings.EnvironmentID, "连接：" + settings.ConnectionID}, err
}

func ValidateProfileSelection(agent string, codexPayload, orcaPayload, remotePayload []byte) error {
	codex, err := codexworker.DecodeProfileConfig(codexPayload)
	if err != nil {
		return err
	}
	orca, err := orcaworker.DecodeProfileConfig(orcaPayload)
	if err != nil {
		return err
	}
	remoteProfile, err := DecodeProfileConfig(remotePayload)
	if err != nil {
		return err
	}
	selected := Selection{Agent: agent, Model: codex.Model, ReasoningEffort: codex.ReasoningEffort, Executable: orca.Executable, Shell: orca.Shell, WaitTimeout: orca.WaitTimeout,
		Settings: &SelectionSettings{Settings: Settings{ConnectionID: remoteProfile.ConnectionID, EnvironmentID: remoteProfile.EnvironmentID, RepositorySelector: remoteProfile.RepositorySelector,
			CoordinatorTerminal: remoteProfile.CoordinatorTerminal, ExpectedCodexVersion: remoteProfile.ExpectedCodexVersion, PollInterval: remoteProfile.PollInterval},
			RemoteCancelAvailable: remoteProfile.RemoteCancelAvailable, CapabilityReportVersion: remoteProfile.CapabilityReportVersion}}
	if err := selected.Validate(); err != nil {
		return err
	}
	return nil
}

// ProfileConfig is owned by the remote implementation codec.
type ProfileConfig struct {
	ConnectionID            string `json:"connection_id"`
	EnvironmentID           string `json:"environment_id"`
	RepositorySelector      string `json:"repository_selector"`
	CoordinatorTerminal     string `json:"coordinator_terminal"`
	ExpectedCodexVersion    string `json:"expected_codex_version,omitempty"`
	PollInterval            string `json:"poll_interval"`
	CapabilityReportVersion int    `json:"capability_report_version"`
	RemoteCancelAvailable   bool   `json:"remote_cancel_available"`
}

type profileConfigWire struct {
	ConnectionID            string `json:"connection_id"`
	EnvironmentID           string `json:"environment_id"`
	RepositorySelector      string `json:"repository_selector"`
	CoordinatorTerminal     string `json:"coordinator_terminal"`
	ExpectedCodexVersion    string `json:"expected_codex_version,omitempty"`
	PollInterval            string `json:"poll_interval"`
	CapabilityReportVersion int    `json:"capability_report_version"`
	RemoteCancelAvailable   *bool  `json:"remote_cancel_available"`
}

func (c *ProfileConfig) UnmarshalJSON(data []byte) error {
	var wire profileConfigWire
	if err := executionbackend.DecodeStrictJSON(data, &wire, "remote profile configuration has trailing JSON"); err != nil {
		return err
	}
	if wire.RemoteCancelAvailable == nil {
		return errors.New("remote_cancel_available must be explicitly set to false")
	}
	*c = ProfileConfig{ConnectionID: wire.ConnectionID, EnvironmentID: wire.EnvironmentID, RepositorySelector: wire.RepositorySelector,
		CoordinatorTerminal: wire.CoordinatorTerminal, ExpectedCodexVersion: wire.ExpectedCodexVersion, PollInterval: wire.PollInterval,
		CapabilityReportVersion: wire.CapabilityReportVersion, RemoteCancelAvailable: *wire.RemoteCancelAvailable}
	return c.Validate()
}

func DecodeProfileConfig(payload []byte) (ProfileConfig, error) {
	var config ProfileConfig
	if err := executionbackend.DecodeStrictJSON(payload, &config, "remote profile configuration has trailing JSON"); err != nil {
		return ProfileConfig{}, err
	}
	if err := config.Validate(); err != nil {
		return ProfileConfig{}, err
	}
	return config, nil
}

func EncodeProfileConfig(config ProfileConfig) ([]byte, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(config)
}

func (c ProfileConfig) Validate() error {
	if strings.TrimSpace(c.ConnectionID) == "" || strings.TrimSpace(c.EnvironmentID) == "" || strings.TrimSpace(c.RepositorySelector) == "" {
		return errors.New("remote profile requires fixed connection, environment and repository identities")
	}
	if c.RemoteCancelAvailable {
		return errors.New("remote cancellation is fixed unavailable")
	}
	if strings.TrimSpace(c.PollInterval) == "" {
		return errors.New("remote poll_interval must be explicit")
	}
	if duration, err := time.ParseDuration(c.PollInterval); err != nil || duration <= 0 {
		return errors.New("remote poll_interval must be a positive duration")
	}
	if c.CapabilityReportVersion != 1 {
		return errors.New("remote capability_report_version must explicitly pin version 1")
	}
	return nil
}
