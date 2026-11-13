package manualverification

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
	"unicode"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

const MaxBytes = 64 << 10
const Port = "${PORT}"

var checkID = regexp.MustCompile(`^[A-Z][A-Z0-9_-]{0,31}$`)
var executableName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.+-]*$`)

type Definition struct {
	Version int     `json:"version"`
	Launch  Launch  `json:"launch"`
	Checks  []Check `json:"checks"`
}

type Launch struct {
	Program string   `json:"program"`
	Args    []string `json:"args"`
	Cwd     string   `json:"cwd,omitempty"`
	URL     string   `json:"url,omitempty"`
}

type Check struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Steps    []string `json:"steps"`
	Expected string   `json:"expected"`
}

type Snapshot struct {
	Path       string     `json:"path"`
	Commit     string     `json:"commit"`
	SHA256     string     `json:"sha256"`
	Definition Definition `json:"definition"`
}

func CompanionPath(plan string) (string, error) {
	if !relativePath(plan) || !strings.HasSuffix(path.Base(plan), ".plan.md") || path.Base(plan) == ".plan.md" {
		return "", fmt.Errorf("Plan must be a normalized repository-relative .plan.md file")
	}
	return strings.TrimSuffix(plan, ".plan.md") + ".manual-checks.json", nil
}

func Freeze(name, commit string, raw []byte) (*Snapshot, error) {
	definition, err := Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	digest := sha256.Sum256(raw)
	return &Snapshot{Path: name, Commit: commit, SHA256: hex.EncodeToString(digest[:]), Definition: definition}, nil
}

func Decode(raw []byte) (Definition, error) {
	var definition Definition
	if len(raw) == 0 || len(raw) > MaxBytes {
		return definition, fmt.Errorf("manual checks must be nonempty JSON of at most %d bytes", MaxBytes)
	}
	canonical, err := runnercontract.CanonicalJSON(raw)
	if err != nil {
		return definition, err
	}
	var shape any
	if err := json.Unmarshal(canonical, &shape); err != nil {
		return definition, err
	}
	if err := validateShape(shape, "$", definitionFields); err != nil {
		return definition, err
	}
	if err := json.Unmarshal(canonical, &definition); err != nil {
		return definition, err
	}
	return definition, definition.validate()
}

// Exact field names and types prevent encoding/json's case-folding and null
// handling from accepting documents outside the public format.
type fields map[string]any

var definitionFields = fields{
	"version": float64(0),
	"launch":  fields{"program": "", "args": []any{""}, "cwd": "", "url": ""},
	"checks":  []any{fields{"id": "", "title": "", "steps": []any{""}, "expected": ""}},
}

func validateShape(value any, name string, expected any) error {
	switch expected := expected.(type) {
	case fields:
		if object, ok := value.(map[string]any); ok {
			for key, value := range object {
				field, exists := expected[key]
				if !exists {
					return fmt.Errorf("%s.%s: unknown field", name, key)
				}
				if err := validateShape(value, name+"."+key, field); err != nil {
					return err
				}
				if s, ok := value.(string); ok && s == "" && (key == "cwd" || key == "url") {
					return fmt.Errorf("%s.%s: omit this field or provide a nonempty value", name, key)
				}
			}
			return nil
		}
	case []any:
		if array, ok := value.([]any); ok {
			for index, item := range array {
				if err := validateShape(item, fmt.Sprintf("%s[%d]", name, index), expected[0]); err != nil {
					return err
				}
			}
			return nil
		}
	case string:
		if _, ok := value.(string); ok {
			return nil
		}
	case float64:
		if _, ok := value.(float64); ok {
			return nil
		}
	}
	return fmt.Errorf("%s: invalid JSON type", name)
}

func (d Definition) validate() error {
	if d.Version != 1 {
		return fmt.Errorf("version: expected integer 1")
	}
	launch := d.Launch
	if len(launch.Program) > 256 || !(executableName.MatchString(launch.Program) || strings.HasPrefix(launch.Program, "./") && relativePath(launch.Program[2:])) {
		return fmt.Errorf("launch.program: expected a system executable name or a normalized ./ path")
	}
	if len(launch.Cwd) > 512 || launch.Cwd != "" && launch.Cwd != "." && !relativePath(launch.Cwd) {
		return fmt.Errorf("launch.cwd: expected a normalized relative directory")
	}
	if launch.Args == nil || len(launch.Args) > 64 {
		return fmt.Errorf("launch.args: expected an array of up to 64 strings")
	}
	usesPort := false
	for i, arg := range launch.Args {
		if len(arg) > 2048 || strings.ContainsFunc(arg, unicode.IsControl) || invalidPlaceholder(arg) {
			return fmt.Errorf("launch.args[%d]: exceeds the limit or contains a control character or invalid placeholder", i)
		}
		usesPort = usesPort || strings.Contains(arg, Port)
	}
	if launch.URL != "" && (!usesPort || !validLaunchURL(launch.URL)) {
		return fmt.Errorf("launch.url: expected http://127.0.0.1:${PORT} with the same port used in launch.args")
	}
	if len(d.Checks) < 1 || len(d.Checks) > 32 {
		return fmt.Errorf("checks: expected 1 to 32 items")
	}
	seen := map[string]bool{}
	for i, check := range d.Checks {
		if !checkID.MatchString(check.ID) || seen[check.ID] {
			return fmt.Errorf("checks[%d].id: invalid or duplicate identifier", i)
		}
		seen[check.ID] = true
		if !validText(check.Title, 120) {
			return fmt.Errorf("checks[%d].title: expected nonblank text of at most 120 bytes", i)
		}
		if len(check.Steps) < 1 || len(check.Steps) > 20 {
			return fmt.Errorf("checks[%d].steps: expected 1 to 20 steps", i)
		}
		for j, step := range check.Steps {
			if !validText(step, 1024) {
				return fmt.Errorf("checks[%d].steps[%d]: expected nonblank text of at most 1024 bytes", i, j)
			}
		}
		if !validText(check.Expected, 2048) {
			return fmt.Errorf("checks[%d].expected: expected nonblank text of at most 2048 bytes", i)
		}
	}
	return nil
}

func Encode(raw json.RawMessage) ([]byte, error) {
	if _, err := Decode(raw); err != nil {
		return nil, err
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return nil, err
	}
	if compact.Len() >= MaxBytes {
		return nil, fmt.Errorf("encoded manual checks exceed %d bytes", MaxBytes)
	}
	return append(compact.Bytes(), '\n'), nil
}

func relativePath(value string) bool {
	return value != "" && value != "." && value != ".." && path.Clean(value) == value && !path.IsAbs(value) &&
		!strings.HasPrefix(value, "../") && !strings.ContainsAny(value, "\\:") &&
		!strings.Contains(value, "${") && !strings.ContainsFunc(value, unicode.IsControl)
}

func invalidPlaceholder(value string) bool {
	return strings.Contains(strings.ReplaceAll(value, Port, ""), "${")
}

func validLaunchURL(value string) bool {
	const prefix = "http://127.0.0.1:" + Port
	if len(value) > 1024 || !strings.HasPrefix(value, prefix) || strings.Count(value, Port) != 1 || invalidPlaceholder(value) || strings.ContainsAny(value, "#\\ ") || strings.ContainsFunc(value, unicode.IsControl) {
		return false
	}
	parsed, err := url.Parse(strings.Replace(value, Port, "12345", 1))
	return err == nil && parsed.Host == "127.0.0.1:12345" && parsed.User == nil
}

func validText(value string, max int) bool {
	return len(value) <= max && strings.TrimSpace(value) != "" && !strings.ContainsRune(value, 0)
}
