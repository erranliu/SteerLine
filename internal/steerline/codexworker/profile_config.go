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

// NormalizeProfilePayload owns the native defaults and non-secret projection.
func NormalizeProfilePayload(payload []byte, policy executionapi.ExecutionPolicy, goos, host, agent string) ([]byte, []string, error) {
	settings, err := DecodeProfileConfig(payload)
	if err != nil {
		return nil, nil, err
	}
	if settings.Executable == "" {
		return nil, nil, errors.New("Codex implementation requires an executable")
	}
	if settings.Model != strings.TrimSpace(settings.Model) || strings.ContainsAny(settings.Model, "\r\n\x00") {
		return nil, nil, errors.New("invalid model")
	}
	display := []string{}
	if host == "orca_supervised" {
		if err := ValidateManagedArgs(settings.Args); err != nil {
			return nil, nil, err
		}
		if settings.Profile != "" || settings.WindowsSandbox != "" {
			return nil, nil, errors.New("remote Codex profile does not accept local profile or Windows sandbox overrides")
		}
		if settings.ReasoningEffort != "" && !ValidReasoningEffort(settings.ReasoningEffort) {
			return nil, nil, errors.New("invalid reasoning_effort")
		}
	} else {
		settings, err = NormalizeProfileConfig(settings, policy, goos, host, agent)
		if err != nil {
			return nil, nil, err
		}
		permissions, err := CompilePolicy(policy, goos, host, agent, settings.WindowsSandbox, settings.Profile, settings.Args)
		if err != nil {
			return nil, nil, err
		}
		display = append(display, "权限："+permissions.Sandbox)
		if permissions.WindowsSandbox != "" {
			display = append(display, "Windows 权限："+permissions.WindowsSandbox)
		}
	}
	if settings.Model != "" {
		display = append(display, "Model："+settings.Model)
	}
	if settings.ReasoningEffort != "" {
		display = append(display, "Reasoning effort："+settings.ReasoningEffort)
	}
	encoded, err := EncodeProfileConfig(settings)
	return encoded, display, err
}

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
