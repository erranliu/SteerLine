package codexworker

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

const ProfileCodecKey = "codex.worker-profile"

// ProfileConfig is the implementation-owned, normalized worker configuration.
type ProfileConfig struct {
	Executable      string   `json:"executable"`
	Args            []string `json:"args"`
	Profile         string   `json:"profile,omitempty"`
	WindowsSandbox  string   `json:"windows_sandbox,omitempty"`
	Model           string   `json:"model,omitempty"`
	ReasoningEffort string   `json:"reasoning_effort,omitempty"`
}

func DecodeProfileConfig(payload []byte) (ProfileConfig, error) {
	var config ProfileConfig
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return ProfileConfig{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return ProfileConfig{}, errors.New("Codex profile configuration has trailing JSON")
	}
	return config, nil
}

func EncodeProfileConfig(config ProfileConfig) ([]byte, error) {
	config.Args = append([]string(nil), config.Args...)
	return json.Marshal(config)
}

func NormalizeProfileConfig(config ProfileConfig, policy executionapi.ExecutionPolicy, goos, host, agent string) (ProfileConfig, error) {
	if err := ValidateManagedArgs(config.Args); err != nil {
		return ProfileConfig{}, err
	}
	permissions, err := CompilePolicy(policy, goos, host, agent, config.WindowsSandbox, config.Profile, config.Args)
	if err != nil {
		return ProfileConfig{}, err
	}
	config.WindowsSandbox = permissions.WindowsSandbox
	if config.Model != strings.TrimSpace(config.Model) || strings.ContainsAny(config.Model, "\r\n\x00") {
		return ProfileConfig{}, errors.New("invalid model")
	}
	if config.ReasoningEffort != "" && !ValidReasoningEffort(config.ReasoningEffort) {
		return ProfileConfig{}, errors.New("invalid reasoning_effort")
	}
	if host == "local" && agent == "codex" {
		config.ReasoningEffort = LocalReasoningEffortDefault(config.ReasoningEffort)
	}
	return config, nil
}
