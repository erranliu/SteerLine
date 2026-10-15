package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	feedback "github.com/erranliu/SteerLine/internal/testkit/feedback"
)

func TestL2RequiresRealDeliveryEvidence(t *testing.T) {
	entry := e2eScenario{ID: "smoke.local-codex", Level: "L2", Env: "live", Chain: "local-codex", Test: "TestSmokeLocalCodex"}
	good := feedback.LiveDeliveryProof{RunnerVersion: "0.1", RunnerSHA256: strings.Repeat("a", 64), RunID: "original-run", CandidateID: "original-candidate", FinalSources: []string{"implement", "report", "publish"}, ArtifactCommit: strings.Repeat("b", 40), Artifact: "real model delivery", PRURL: "https://github.com/erranliu/SteerlineTest/pull/1", Repository: "erranliu/SteerlineTest", Base: "codex/e2e/owned/smoke.local-codex/base", Head: "codex/owned", HeadSHA: strings.Repeat("b", 40), MergeSHA: strings.Repeat("c", 40), Merged: true, DeliveryCandidate: "original-candidate", DeliveryPR: "https://github.com/erranliu/SteerlineTest/pull/1", DeliveryCount: 1, SuccessorReady: true, SuccessorUnstarted: true}
	for _, tc := range []struct {
		name   string
		change func(*feedback.LiveDeliveryProof)
		pass   bool
	}{
		{"complete real chain", func(*feedback.LiveDeliveryProof) {}, true},
		{"missing Runner", func(p *feedback.LiveDeliveryProof) { p.RunnerSHA256 = "" }, false},
		{"missing stage", func(p *feedback.LiveDeliveryProof) { p.FinalSources = p.FinalSources[:2] }, false},
		{"wrong artifact", func(p *feedback.LiveDeliveryProof) { p.Artifact = "model says done" }, false},
		{"wrong head", func(p *feedback.LiveDeliveryProof) { p.HeadSHA = strings.Repeat("d", 40) }, false},
		{"wrong repo", func(p *feedback.LiveDeliveryProof) { p.Repository = "e2e/project" }, false},
		{"wrong base", func(p *feedback.LiveDeliveryProof) { p.Base = "main" }, false},
		{"wrong candidate", func(p *feedback.LiveDeliveryProof) { p.DeliveryCandidate = "sibling" }, false},
		{"wrong PR", func(p *feedback.LiveDeliveryProof) { p.DeliveryPR = "https://github.com/erranliu/SteerlineTest/pull/2" }, false},
		{"not merged", func(p *feedback.LiveDeliveryProof) { p.Merged = false }, false},
		{"duplicate delivery", func(p *feedback.LiveDeliveryProof) { p.DeliveryCount = 2 }, false},
		{"successor blocked", func(p *feedback.LiveDeliveryProof) { p.SuccessorReady = false }, false},
		{"successor started", func(p *feedback.LiveDeliveryProof) { p.SuccessorUnstarted = false }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proof := good
			tc.change(&proof)
			out := t.TempDir()
			dir := filepath.Join(out, "scenarios", entry.ID)
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := writeJSON(filepath.Join(dir, "scenario.json"), map[string]any{"id": entry.ID, "passed": true, "cleanup_complete": true, "resources": []any{map[string]string{"kind": "project"}}, "live_delivery": proof}); err != nil {
				t.Fatal(err)
			}
			var logs strings.Builder
			for _, e := range []struct{ action, test string }{{"start", ""}, {"run", entry.Test}, {"pass", entry.Test}, {"pass", ""}} {
				raw, _ := json.Marshal(map[string]any{"Time": "2026-10-15T00:00:00Z", "Elapsed": 0, "Action": e.action, "Package": "p", "Test": e.test})
				logs.Write(raw)
				logs.WriteByte('\n')
			}
			g := scopedGroup{Packages: []string{"p"}, Summary: feedback.Parse([]byte(logs.String()), 0)}
			if err := checkE2EGroup(g, []e2eScenario{entry}, out); (err == nil) != tc.pass {
				t.Fatalf("invalid live evidence decision: %v", err)
			}
		})
	}
}
