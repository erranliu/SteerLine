package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"path"
	"strings"
	"unicode/utf8"
)

const MaxDocumentBytes = 1 << 20

type Arrangement struct {
	Tests []string `json:"tests"`
}

func Decode(raw []byte, target any) error {
	if len(raw) == 0 || len(raw) > MaxDocumentBytes || !utf8.Valid(raw) {
		return errors.New("test document must be bounded nonempty UTF-8")
	}
	canonical, err := runnercontract.CanonicalJSON(raw)
	if err != nil {
		return err
	}
	d := json.NewDecoder(strings.NewReader(string(canonical)))
	d.DisallowUnknownFields()
	return d.Decode(target)
}
func ArrangementPath(plan string) (string, error) {
	if path.Clean(plan) != plan || strings.ContainsAny(plan, "\\\x00") || path.IsAbs(plan) || strings.Contains(plan, ":") || strings.HasPrefix(plan, "../") || !strings.HasSuffix(plan, ".plan.md") {
		return "", errors.New("Plan must be a normalized repository-relative .plan.md file")
	}
	return strings.TrimSuffix(plan, ".plan.md") + ".tests.json", nil
}
func ParseCatalogue(raw []byte) (Catalogue, error) {
	var c Catalogue
	if err := Decode(raw, &c); err != nil {
		return c, err
	}
	if c.Tests == nil {
		return c, errors.New("tests must be an array")
	}
	seen := map[string]bool{}
	for _, d := range c.Tests {
		if strings.TrimSpace(d.ID) == "" || seen[d.ID] || strings.TrimSpace(d.Name) == "" || strings.TrimSpace(d.Environment) == "" || strings.TrimSpace(d.Command) == "" || strings.TrimSpace(d.Check) == "" {
			return c, errors.New("test definitions require unique IDs and nonempty fields")
		}
		if d.Level != "L1" && d.Level != "L2" && d.Level != "L3" && d.Level != "L4" {
			return c, errors.New("invalid test level: " + d.Level)
		}
		seen[d.ID] = true
	}
	return c, nil
}
func Select(raw []byte, c Catalogue) ([]Definition, error) {
	var a Arrangement
	if err := Decode(raw, &a); err != nil {
		return nil, err
	}
	if a.Tests == nil {
		return nil, errors.New("tests must be an array")
	}
	index := map[string]Definition{}
	for _, d := range c.Tests {
		index[d.ID] = d
	}
	result := []Definition{}
	seen := map[string]bool{}
	for _, id := range a.Tests {
		d, ok := index[id]
		if !ok || seen[id] {
			return nil, fmt.Errorf("unknown or duplicate test ID: %s", id)
		}
		seen[id] = true
		result = append(result, d)
	}
	return result, nil
}
