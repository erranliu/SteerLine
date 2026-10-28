// Package testmanagement owns test documents and result projections, not execution.
package testmanagement

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/testmanagement/model"
)

const CataloguePath = model.CataloguePath
const RulesPath = model.RulesPath
const MaxDocumentBytes = model.MaxDocumentBytes

type Definition = model.Definition
type Catalogue = model.Catalogue

type Arrangement = model.Arrangement

type Snapshot = model.Snapshot

func Decode(raw []byte, target any) error { return model.Decode(raw, target) }
func Digest(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func ContentDigest(raw []byte) string                      { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func ArrangementPath(plan string) (string, error)          { return model.ArrangementPath(plan) }
func ParseCatalogue(raw []byte) (Catalogue, error)         { return model.ParseCatalogue(raw) }
func Select(raw []byte, c Catalogue) ([]Definition, error) { return model.Select(raw, c) }

type Result = model.Result
type Report = model.Report
type RecordFilter = model.RecordFilter

func parseReport(raw []byte) (Report, error) {
	var r Report
	if err := Decode(raw, &r); err != nil {
		return r, err
	}
	if r.ID == "" || r.Sequence == 0 || r.Environment == "" || len(r.Results) == 0 || len(r.Selection) == 0 {
		return r, errors.New("incomplete test report")
	}
	selected := map[string]bool{}
	for _, id := range r.Selection {
		if id == "" || selected[id] {
			return r, errors.New("duplicate or empty test selection")
		}
		selected[id] = true
	}
	seen := map[string]bool{}
	for _, item := range r.Results {
		if !selected[item.TestID] || seen[item.TestID] || r.Definitions[item.TestID] == "" || item.Log == "" {
			return r, errors.New("test report selection/definition/log mismatch")
		}
		if item.Status != "passed" && item.Status != "failed" && item.Status != "incomplete" {
			return r, errors.New("invalid test result status")
		}
		if item.Status != "passed" && strings.TrimSpace(item.Message) == "" {
			return r, errors.New("failed/incomplete results require a reason")
		}
		seen[item.TestID] = true
	}
	if len(seen) != len(selected) {
		return r, errors.New("test report does not cover its selection")
	}
	return r, nil
}

type Row struct {
	Definition
	Status string `json:"status"`
}
type View struct {
	ProgressVersion  uint64     `json:"progress_version"`
	ObservedAt       *time.Time `json:"observed_at,omitempty"`
	CollectionStatus string     `json:"collection_status,omitempty"`
	Commit           string     `json:"commit"`
	Status           string     `json:"status"`
	Diagnostic       string     `json:"diagnostic,omitempty"`
	PlanPath         string     `json:"plan_path"`
	Total            int        `json:"total"`
	Passed           int        `json:"passed"`
	Pending          int        `json:"pending"`
	Rows             []Row      `json:"rows"`
}
