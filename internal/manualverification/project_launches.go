package manualverification

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

const ProjectLaunchPath = "SteerlineFlow/manual-launches.json"

type ProjectLaunch struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Launch Launch `json:"launch"`
}

var projectLaunchID = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

func DecodeProjectLaunches(raw []byte) ([]ProjectLaunch, error) {
	if len(raw) == 0 || len(raw) > MaxBytes {
		return nil, fmt.Errorf("manual launches must be nonempty JSON of at most %d bytes", MaxBytes)
	}
	canonical, err := runnercontract.CanonicalJSON(raw)
	if err != nil {
		return nil, err
	}
	var shape any
	if err := json.Unmarshal(canonical, &shape); err != nil {
		return nil, err
	}
	if err := validateShape(shape, "$", fields{
		"version": float64(0),
		"entries": []any{fields{"id": "", "name": "", "launch": launchFields}},
	}); err != nil {
		return nil, err
	}
	var config struct {
		Version int             `json:"version"`
		Entries []ProjectLaunch `json:"entries"`
	}
	if err := json.Unmarshal(canonical, &config); err != nil {
		return nil, err
	}
	if config.Version != 1 || len(config.Entries) < 1 || len(config.Entries) > 32 {
		return nil, fmt.Errorf("expected version 1 and 1 to 32 entries")
	}
	seen := map[string]bool{}
	for i, entry := range config.Entries {
		if !projectLaunchID.MatchString(entry.ID) || seen[entry.ID] {
			return nil, fmt.Errorf("entries[%d].id: invalid or duplicate identifier", i)
		}
		seen[entry.ID] = true
		if !validText(entry.Name, 120) || strings.ContainsFunc(entry.Name, unicode.IsControl) {
			return nil, fmt.Errorf("entries[%d].name: expected nonblank text of at most 120 bytes", i)
		}
		if err := entry.Launch.validate(); err != nil {
			return nil, fmt.Errorf("entries[%d]: %w", i, err)
		}
	}
	return config.Entries, nil
}
