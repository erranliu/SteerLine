package orcaworker

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

const ProfileCodecKey = "orca.worker-profile"

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
	return encoded, nil, err
}

func ValidateProfileSelection(agent string, codexPayload, orcaPayload []byte) error {
	codex, err := codexworker.DecodeProfileConfig(codexPayload)
	if err != nil {
		return err
	}
	orca, err := DecodeProfileConfig(orcaPayload)
	if err != nil {
		return err
	}
	if agent != "codex" && (codex.Model != "" || codex.ReasoningEffort != "") {
		return errors.New("Orca model, profile and reasoning_effort overrides require agent=codex")
	}
	if codex.Model != "" && orca.Shell != "powershell.exe" && orca.Shell != "pwsh.exe" {
		return errors.New("fixed Orca Codex launch requires powershell.exe or pwsh.exe")
	}
	if codex.Model != "" && codex.Executable == "" {
		return errors.New("fixed Orca Codex launch requires codex.executable")
	}
	return nil
}

// ProfileConfig is the native Orca command configuration accepted by its codec.
type ProfileConfig struct {
	Executable   string   `json:"executable"`
	Args         []string `json:"args"`
	Shell        string   `json:"shell"`
	WaitTimeout  string   `json:"wait_timeout"`
	KeepTerminal bool     `json:"keep_terminal"`
	Activate     bool     `json:"activate"`
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
