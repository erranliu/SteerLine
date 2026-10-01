package orcaworker

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

const ProfileCodecKey = "orca.worker-profile"

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
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return ProfileConfig{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return ProfileConfig{}, errors.New("Orca profile configuration has trailing JSON")
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

func ValidateProfileConfig(c ProfileConfig) error { return c.Validate() }

func ValidateLocalTargetArgs(args []string) error {
	for _, arg := range args {
		if arg == "--environment" || arg == "--pairing-code" || strings.HasPrefix(arg, "--environment=") || strings.HasPrefix(arg, "--pairing-code=") {
			return errors.New("local Orca target cannot be redirected through orca.args; remote execution_policy requires the remote protocol adapter")
		}
	}
	return nil
}
