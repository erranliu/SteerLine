// Package executionconfig contains configuration values only; it cannot probe,
// load files, or start an execution.
package executionconfig

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"

	"github.com/erranliu/SteerLine/internal/developerconfig"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

type Pin struct {
	Executable string `json:"executable"`
	Version    string `json:"version"`
	SHA256     string `json:"sha256"`
}
type File[P any] struct {
	Developer developerconfig.Config `json:"developer"`

	Version                int             `json:"version"`
	Runner                 P               `json:"runner"`
	DefaultProfile         string          `json:"default_execution_profile"`
	MaxGenerateConcurrency *int            `json:"max_generate_concurrency,omitempty"`
	UsagePolicy            json.RawMessage `json:"usage_policy,omitempty"`
	Profiles               []Profile       `json:"execution_profiles"`
	SteerAgent             *SteerAgent     `json:"steer_agent,omitempty"`
}

type SteerAgent struct {
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
}

func SteerAgentSettings(value *SteerAgent) (SteerAgent, error) {
	if value == nil {
		return SteerAgent{Model: "gpt-6.1-sol", ReasoningEffort: "high"}, nil
	}
	if strings.TrimSpace(value.Model) != value.Model || value.Model == "" || strings.ContainsAny(value.Model, "\r\n\x00") {
		return *value, errors.New("steer_agent.model must be an explicit model identifier")
	}
	switch value.ReasoningEffort {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max":
	default:
		return *value, errors.New("steer_agent.reasoning_effort is invalid")
	}
	return *value, nil
}

type Profile struct {
	ID        string                        `json:"id"`
	Name      string                        `json:"name"`
	Execution runnercontract.ExecutionInput `json:"execution"`
	Workflow  runnercontract.Workflow       `json:"workflow"`
}

// SteerAgentBase reuses only an already configured local Codex transport.
// Its model and permission are replaced through Runner's public compose API.
// Remote executables and other agents are never used as a local fallback.
func SteerAgentBase(profiles []Profile) *runnercontract.ExecutionInput {
	for _, profile := range profiles {
		if profile.Execution.Adapter == "codex-app-server" && profile.Execution.Target.Kind == "local" {
			copy := profile.Execution
			copy.Settings = append(json.RawMessage(nil), copy.Settings...)
			return &copy
		}
	}
	return nil
}

func (p Profile) Digest() string { return runnercontract.ConfigurationDigest(p.Execution, p.Workflow) }

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

func Validate(version int, defaultID string, profiles []Profile) error {
	if version != 3 || len(profiles) == 0 {
		return errors.New("execution config requires version 3 and at least one profile")
	}
	seen := map[string]bool{}
	for _, p := range profiles {
		if !idPattern.MatchString(p.ID) || strings.TrimSpace(p.Name) == "" || seen[p.ID] || p.Execution.Adapter == "" || p.Execution.Target.Kind == "" || len(p.Execution.Settings) == 0 || !json.Valid(p.Execution.Settings) || p.Workflow.ID == "" || p.Workflow.Version <= 0 || p.Digest() == "" {
			return errors.New("execution profile is incomplete or duplicated")
		}
		seen[p.ID] = true
	}
	if !seen[defaultID] {
		return errors.New("default_execution_profile must select a configured profile")
	}
	return nil
}
func Decode(raw []byte, value any) error {
	raw = bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})
	if _, err := runnercontract.CanonicalJSON(raw); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	d.UseNumber()
	if err := d.Decode(value); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("one JSON value required")
	}
	return nil
}
func Digest(raw []byte) string { v := sha256.Sum256(raw); return hex.EncodeToString(v[:]) }

// GenerateConcurrency is a caller scheduling budget, frozen at application start.
func GenerateConcurrency(value *int) (int, error) {
	if value == nil {
		return 5, nil
	}
	if *value <= 0 {
		return 0, errors.New("max_generate_concurrency must be a positive integer")
	}
	return *value, nil
}
