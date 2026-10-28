package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"strings"
)

const RulesPath = "SteerlineFlow/testing.md"
const CataloguePath = "SteerlineFlow/testing/test-list.json"
const WriterPath = "tools/testing/RuntimeProgress.ps1"

// TestingFacts projects only the immutable arrangement identity and selected IDs.
// Fixed instructions belong to prompts; full definitions remain authoritative.
func TestingFacts(snapshot []byte) (json.RawMessage, error) {
	var full Snapshot
	if json.Unmarshal(snapshot, &full) != nil || full.Status != "ready" {
		return nil, errors.New("ready frozen test arrangement is required")
	}
	ids := make([]string, 0, len(full.Definitions))
	for _, d := range full.Definitions {
		ids = append(ids, d.ID)
	}
	sum := sha256.Sum256(snapshot)
	view, _ := json.Marshal(struct {
		IDs            []string `json:"tests"`
		Plan           string   `json:"plan_path"`
		Commit         string   `json:"commit"`
		Digest         string   `json:"digest"`
		SnapshotDigest string   `json:"snapshot_sha256"`
		Generation     string   `json:"generation_id"`
		Inputs         string   `json:"inputs"`
	}{ids, full.PlanPath, full.Commit, full.Digest, hex.EncodeToString(sum[:]), full.GenerationID, full.Inputs})
	return view, nil
}

// TestReference admits only real worktree paths beside the contract documents.
func TestReference(doc runnercontract.Document, plan, commit string, ready bool) bool {
	return ready && strings.HasSuffix(plan, ".plan.md") && doc.Kind == "" && doc.Content == nil && doc.Commit == "" && (doc.Path == CataloguePath || doc.Path == RulesPath || doc.Path == WriterPath)
}
