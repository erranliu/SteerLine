// Package executionconfig contains configuration values only; it cannot probe,
// load files, or start an execution.
package executionconfig

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"io"
	"regexp"
	"strings"
)

type Pin struct {
	Executable string `json:"executable"`
	Version    string `json:"version"`
	SHA256     string `json:"sha256"`
}
type File[P any] struct {
	Version                int       `json:"version"`
	Runner                 P         `json:"runner"`
	DefaultProfile         string    `json:"default_execution_profile"`
	MaxGenerateConcurrency *int      `json:"max_generate_concurrency,omitempty"`
	Profiles               []Profile `json:"execution_profiles"`
}
type Profile struct {
	ID        string                        `json:"id"`
	Name      string                        `json:"name"`
	Execution runnercontract.ExecutionInput `json:"execution"`
	Workflow  runnercontract.Workflow       `json:"workflow"`
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
