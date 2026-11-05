package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

const ProjectEntryPath = "SteerlineFlow/environment-project.json"

// ProjectEntry locates project-owned policy and the one formal test catalogue.
// It contains neither execution settings nor environment qualification state.
type ProjectEntry struct {
	Version          int      `json:"version"`
	Requirements     []string `json:"requirements"`
	TestingCatalogue string   `json:"testing_catalogue"`
}

func ProjectFilePath(p string) bool {
	return p != "." && fs.ValidPath(p) && !strings.ContainsAny(p, "\\:\x00*?[]")
}

func ParseProjectEntry(raw []byte) (ProjectEntry, error) {
	var out ProjectEntry
	if len(raw) == 0 || len(raw) > 1<<20 || !utf8.Valid(raw) {
		return out, errors.New("project entry must be bounded UTF-8")
	}
	canonical, err := runnercontract.CanonicalJSON(raw)
	if err != nil {
		return out, err
	}
	d := json.NewDecoder(bytes.NewReader(canonical))
	d.DisallowUnknownFields()
	if err := d.Decode(&out); err != nil {
		return out, err
	}
	if out.Version != 1 || out.Requirements == nil || out.TestingCatalogue == "" {
		return out, errors.New("project entry requires version 1, requirements and testing_catalogue")
	}
	seen := map[string]bool{}
	for _, p := range append(append([]string{}, out.Requirements...), out.TestingCatalogue) {
		if !ProjectFilePath(p) || p == ProjectEntryPath || p == "SteerlineProjectGuideDoc/environment-adaptation.md" || p == "SteerlineFlow/environment.md" || p == "SteerlineFlow/scripts/verify-environment.ps1" || p == "SteerlineFlow/scripts/verify-environment.sh" || strings.HasPrefix(p, "SteerlineFlow/environments/") || p == "SteerlineFlow/environment-generation.md" || p == "SteerlineFlow/environment-execution.md" {
			return out, errors.New("project entry requires finite project-owned regular file paths")
		}
	}
	for _, p := range out.Requirements {
		if seen[p] {
			return out, errors.New("duplicate project requirement path")
		}
		seen[p] = true
	}
	sort.Strings(out.Requirements)
	return out, nil
}

func ReadProjectEntry(read func(string) ([]byte, error)) (ProjectEntry, error) {
	raw, err := read(ProjectEntryPath)
	if err != nil {
		return ProjectEntry{}, err
	}
	return ParseProjectEntry(raw)
}

// Paths is the finite common source set; callers read it at their frozen commit.
func (p ProjectEntry) Paths() []string {
	paths := []string{ProjectEntryPath}
	seen := map[string]bool{ProjectEntryPath: true}
	for _, name := range append(append([]string{}, p.Requirements...), p.TestingCatalogue) {
		if !seen[name] {
			paths = append(paths, name)
			seen[name] = true
		}
	}
	return paths
}
