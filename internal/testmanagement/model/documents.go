package model

import (
	"encoding/json"
	"errors"

	"github.com/erranliu/SteerLine/internal/runnercontract"
	"strings"
)

const RulesPath = "SteerlineFlow/testing.md"
const CataloguePath = "SteerlineFlow/testing/test-list.json"
const WriterPath = "tools/testing/RuntimeProgress.ps1"
const ProtocolPath = "SteerlineFlow/testing/runtime-progress.md"

// FrozenArrangement serializes the original generated selection for the Runner.
func FrozenArrangement(plan Snapshot) ([]byte, error) {
	ids := make([]string, 0, len(plan.Definitions))
	for _, definition := range plan.Definitions {
		ids = append(ids, definition.ID)
	}
	return json.Marshal(Arrangement{Tests: ids})
}

// ValidateArrangement compares a selection with the original
// frozen definitions, including order and note. It cannot select new tests.
func ValidateArrangement(plan Snapshot, raw []byte, catalogue Catalogue) error {
	selected, err := Select(raw, catalogue)
	if err != nil {
		return err
	}
	if len(selected) != len(plan.Definitions) {
		return errors.New("fixed arrangement selection differs from frozen tests")
	}
	for i, d := range selected {
		if d != plan.Definitions[i] {
			return errors.New("fixed arrangement definition/order differs from frozen tests")
		}
	}
	return nil
}
func TestReference(doc runnercontract.Document, plan string, project ProjectEntry) bool {
	if project.Version != 1 || !strings.HasSuffix(plan, ".plan.md") || doc.Kind != "" || doc.Content != nil || doc.Commit != "" {
		return false
	}
	for _, path := range append(project.Paths(), WriterPath, ProtocolPath) {
		if doc.Path == path {
			return true
		}
	}
	return false
}
