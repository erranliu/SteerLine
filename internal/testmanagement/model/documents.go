package model

import (
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"strings"
)

const RulesPath = "SteerlineFlow/testing.md"
const CataloguePath = "SteerlineFlow/testing/test-list.json"
const WriterPath = "tools/testing/RuntimeProgress.ps1"
const ProtocolPath = "SteerlineFlow/testing/runtime-progress.md"

// ValidateArrangement compares a real versioned selection with the original
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
func TestReference(doc runnercontract.Document, plan, commit string, ready bool) bool {
	arrangement, err := ArrangementPath(plan)
	return err == nil && ready && strings.HasSuffix(plan, ".plan.md") && doc.Kind == "" && doc.Content == nil && doc.Commit == "" && (doc.Path == CataloguePath || doc.Path == RulesPath || doc.Path == WriterPath || doc.Path == ProtocolPath || doc.Path == arrangement)
}
