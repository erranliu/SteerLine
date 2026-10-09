package orcaworker

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

const ProfileCodecKey = "orca.worker-profile"
const ProfileCodecVersion = 1

func NormalizeProfilePayload(payload []byte, host string) ([]byte, []string, error) {
	settings, err := DecodeProfileConfig(payload)
	if err != nil {
		return nil, nil, err
	}
	if err := settings.Validate(); err != nil {
		return nil, nil, err
	}
	if host == "orca" {
		if err := ValidateLocalTargetArgs(settings.Args); err != nil {
			return nil, nil, err
		}
	}
	encoded, err := EncodeProfileConfig(settings)
	return encoded, []string{"权限：由 Orca 原生 Agent Permissions 设置管理"}, err
}

func ValidateProfileSelection(agent string, payload []byte) error {
	if strings.TrimSpace(agent) == "" || agent != strings.TrimSpace(agent) || strings.HasPrefix(agent, "-") || strings.ContainsAny(agent, " \t\r\n") {
		return errors.New("Orca agent must be a nonempty configuration value")
	}
	profile, err := DecodeProfileConfig(payload)
	if err != nil {
		return err
	}
	return profile.Validate()
}

// ProfileConfig is the native Orca command configuration accepted by its codec.
type ProfileConfig struct {
	Model           string   `json:"model,omitempty"`
	ReasoningEffort string   `json:"reasoning_effort,omitempty"`
	Executable      string   `json:"executable"`
	Args            []string `json:"args"`
	Shell           string   `json:"shell"`
	WaitTimeout     string   `json:"wait_timeout"`
	KeepTerminal    bool     `json:"keep_terminal"`
	Activate        bool     `json:"activate"`
}

// NativeTargetID is shared by target freezing and lifecycle validation. Empty
// and omitted argument lists describe the same command, including after cloning.
func NativeTargetID(config ProfileConfig) string {
	return "orca:" + digestValue(struct {
		Executable string
		Args       []string
	}{config.Executable, append([]string(nil), config.Args...)})[:24]
}

func DecodeProfileConfig(payload []byte) (ProfileConfig, error) {
	var config ProfileConfig
	if err := executionbackend.DecodeStrictJSON(payload, &config, "Orca profile configuration has trailing JSON"); err != nil {
		return ProfileConfig{}, err
	}
	return config, nil
}

func EncodeProfileConfig(config ProfileConfig) ([]byte, error) {
	return json.Marshal(config)
}

func (c ProfileConfig) Validate() error {
	if c.Model != strings.TrimSpace(c.Model) || c.ReasoningEffort != strings.TrimSpace(c.ReasoningEffort) || strings.HasPrefix(c.Model, "-") || strings.HasPrefix(c.ReasoningEffort, "-") {
		return errors.New("Orca launch preferences must be literal option values")
	}
	if c.ReasoningEffort != "" && c.Model == "" {
		return errors.New("Orca reasoning_effort requires model")
	}
	if c.Executable == "" || c.Shell == "" {
		return errors.New("orca.executable and orca.shell are required")
	}
	if timeout, err := time.ParseDuration(c.WaitTimeout); err != nil || timeout <= 0 {
		return errors.New("orca.wait_timeout must be a positive duration")
	}
	return nil
}

func ValidateLocalTargetArgs(args []string) error {
	for _, arg := range args {
		if arg == "--environment" || arg == "--pairing-code" || strings.HasPrefix(arg, "--environment=") || strings.HasPrefix(arg, "--pairing-code=") {
			return errors.New("local Orca target cannot be redirected through orca.args; remote execution requires the remote protocol adapter")
		}
	}
	return nil
}
