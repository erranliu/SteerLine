package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAdvisoryEvidenceRejectsMissingFactsAndBrowserProof(t *testing.T) {
	for _, missing := range append(append([]string{"none", "jobs", "browser-ready", "browser-cancel", "commands", "public-missing", "public-empty", "public-malformed", "screenshot-ready", "screenshot-cancel", "submit-replay"}, advisoryProofFacts...), "malformed") {
		t.Run(missing, func(t *testing.T) {
			dir := t.TempDir()
			write := func(path string, value any) {
				t.Helper()
				file := filepath.Join(dir, path)
				if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
					t.Fatal(err)
				}
				raw, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(file, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			proof := map[string]any{"jobs": 6}
			for _, fact := range advisoryProofFacts {
				proof[fact] = true
			}
			delete(proof, missing)
			write("advisory-proof.json", proof)
			questions, calls, bodies, requests := map[string]string{}, map[string]int{}, map[string]string{}, map[string]any{}
			for _, q := range []string{"ready", "unknown", "failed", "cancel", "after", "source"} {
				questions[q] = q
				calls[q] = 1
				bodies[q] = `{"generate":{"instruction":"full text"}}`
				requests[q] = map[string]any{"command": "submit", "run_id": q}
			}
			if missing == "submit-replay" {
				calls["unknown"] = 2
			}
			if missing != "public-missing" {
				write("advisory-public-requests.json", map[string]any{"advisory_questions": questions, "submit_calls": calls, "bodies": bodies, "requests": requests})
			}
			if missing == "public-empty" {
				write("advisory-public-requests.json", map[string]any{})
			}
			if missing == "public-malformed" {
				if err := os.WriteFile(filepath.Join(dir, "advisory-public-requests.json"), []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, mode := range []string{"ready", "cancel"} {
				if missing == "browser-"+mode {
					continue
				}
				commands := []string{}
				if mode == "cancel" {
					commands = []string{"main_agent_cancel"}
				}
				if missing == "commands" {
					commands = append(commands, "answer")
				}
				if missing != "screenshot-"+mode {
					file := filepath.Join(dir, "browser-"+mode, "advisory-"+mode+".png")
					if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(file, []byte("screenshot"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				write(filepath.Join("browser-"+mode, "proof.json"), map[string]any{"passed": true, "mode": mode, "navigation": true, "context": true, "copy": mode == "ready", "cancel": mode == "cancel", "commands": commands})
			}
			if missing == "malformed" {
				if err := os.WriteFile(filepath.Join(dir, "advisory-proof.json"), []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			err := checkAdvisoryEvidence(dir)
			if (err == nil) != (missing == "none") {
				t.Fatalf("evidence %s: %v", missing, err)
			}
		})
	}
}

func TestAdvisoryRequiresSerialPackageAndFreshWeb(t *testing.T) {
	entries := []e2eScenario{{ID: "runtime.main-agent-advisory", Suite: "runtime", Level: "L4", Env: "controlled"}}
	if e2ePackageParallelism(entries) != 1 || !hasMainAgentScenario(entries) {
		t.Fatal("advisory requires serial browser scenario with fresh Web assets")
	}
}
