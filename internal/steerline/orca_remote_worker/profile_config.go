package orca_remote_worker

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

const ProfileCodecKey = "orca.supervised-profile"

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
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("remote profile configuration has trailing JSON")
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
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return ProfileConfig{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return ProfileConfig{}, errors.New("remote profile configuration has trailing JSON")
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
	if strings.TrimSpace(c.ConnectionID) == "" || strings.TrimSpace(c.EnvironmentID) == "" || strings.TrimSpace(c.RepositorySelector) == "" || strings.TrimSpace(c.CoordinatorTerminal) == "" {
		return errors.New("remote profile requires fixed connection, environment, repository and coordinator identities")
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
