package orca_remote_worker

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

const ProfileCodecKey = "orca.supervised-profile"
const ProfileCodecVersion = 2

func NormalizeProfilePayload(payload []byte) ([]byte, []string, error) {
	settings, err := DecodeProfileConfig(payload)
	if err != nil {
		return nil, nil, err
	}
	encoded, err := EncodeProfileConfig(settings)
	return encoded, []string{"目标环境：" + settings.EnvironmentID, "连接：" + settings.ConnectionID}, err
}

func ValidateProfileSelection(agent string, remotePayload []byte) error {
	if strings.TrimSpace(agent) == "" || agent != strings.TrimSpace(agent) || strings.ContainsAny(agent, "\r\n\x00") || agent == "*" {
		return errors.New("remote Orca requires an opaque nonempty agent name")
	}
	_, err := DecodeProfileConfig(remotePayload)
	return err
}

// ProfileConfig is owned by the remote implementation codec.
type ProfileConfig struct {
	Executable      string   `json:"executable"`
	Args            []string `json:"args,omitempty"`
	Shell           string   `json:"shell"`
	WaitTimeout     string   `json:"wait_timeout"`
	Model           string   `json:"model,omitempty"`
	ReasoningEffort string   `json:"reasoning_effort,omitempty"`

	ConnectionID            string `json:"connection_id"`
	EnvironmentID           string `json:"environment_id"`
	RepositorySelector      string `json:"repository_selector"`
	CoordinatorTerminal     string `json:"coordinator_terminal"`
	PollInterval            string `json:"poll_interval"`
	CapabilityReportVersion int    `json:"capability_report_version"`
	RemoteCancelAvailable   bool   `json:"remote_cancel_available"`
}

type profileConfigWire struct {
	Executable      string   `json:"executable"`
	Args            []string `json:"args,omitempty"`
	Shell           string   `json:"shell"`
	WaitTimeout     string   `json:"wait_timeout"`
	Model           string   `json:"model,omitempty"`
	ReasoningEffort string   `json:"reasoning_effort,omitempty"`

	ConnectionID            string `json:"connection_id"`
	EnvironmentID           string `json:"environment_id"`
	RepositorySelector      string `json:"repository_selector"`
	CoordinatorTerminal     string `json:"coordinator_terminal"`
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
	*c = ProfileConfig{Executable: wire.Executable, Args: wire.Args, Shell: wire.Shell, WaitTimeout: wire.WaitTimeout, Model: wire.Model, ReasoningEffort: wire.ReasoningEffort, ConnectionID: wire.ConnectionID, EnvironmentID: wire.EnvironmentID, RepositorySelector: wire.RepositorySelector,
		CoordinatorTerminal: wire.CoordinatorTerminal, PollInterval: wire.PollInterval,
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
	if c.Executable == "" || c.Shell == "" {
		return errors.New("remote Orca executable and coordinator shell are required")
	}
	if d, err := time.ParseDuration(c.WaitTimeout); err != nil || d <= 0 {
		return errors.New("remote Orca wait_timeout must be positive")
	}
	if c.ReasoningEffort != "" && c.Model == "" {
		return errors.New("Orca effort requires an explicit model")
	}
	if c.Model != strings.TrimSpace(c.Model) || c.ReasoningEffort != strings.TrimSpace(c.ReasoningEffort) || strings.ContainsAny(c.Model+c.ReasoningEffort, "\r\n\x00") {
		return errors.New("invalid opaque Orca launch selection")
	}
	for _, arg := range c.Args {
		if arg == "--environment" || arg == "--pairing-code" || strings.HasPrefix(arg, "--environment=") || strings.HasPrefix(arg, "--pairing-code=") {
			return errors.New("remote Orca home CLI routing belongs to fixed connection settings")
		}
	}

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
