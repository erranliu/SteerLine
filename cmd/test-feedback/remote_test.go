package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/testkit/remotetest"
)

func TestRemoteL4PreciseSelectionAndNoProofFallback(t *testing.T) {
	root, err := e2eRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ id, suite string }{{remotetest.Discovery, "configuration"}, {remotetest.Progress, "testing"}, {remotetest.Environment, "testing"}} {
		selected, err := selectE2E(root, options{level: "L4", e2eEnv: "live", suite: item.suite, scenario: item.id})
		if err != nil || len(selected) != 1 || selected[0].Chain != "orca-remote" {
			t.Fatal(selected, err)
		}
		if _, err = selectE2E(root, options{level: "L3", e2eEnv: "live", suite: item.suite, scenario: item.id}); err == nil {
			t.Fatal("wrong level accepted")
		}
		out := filepath.Join(t.TempDir(), "scenarios", item.id)
		if err = os.MkdirAll(out, 0700); err != nil {
			t.Fatal(err)
		}
		if err = checkRemoteEvidence(item.id, out); err == nil {
			t.Fatal("missing purpose-specific evidence passed")
		}
	}
}

func TestRemoteL4PrerequisiteFailureIsReported(t *testing.T) {
	t.Setenv("STEERLINE_L4_REMOTE_PROFILE", filepath.Join(t.TempDir(), "missing.json"))
	out := t.TempDir()
	if code := runE2E(options{level: "L4", e2eEnv: "live", suite: "configuration", scenario: remotetest.Discovery, out: out}); code == 0 {
		t.Fatal("missing input passed")
	}
	raw, err := os.ReadFile(filepath.Join(out, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report e2eReport
	if err = json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if report.Passed || !report.Cleanup || len(report.Errors) != 1 || !strings.Contains(report.Errors[0], "missing.json") || len(report.Groups) != 0 {
		t.Fatal("prerequisite failure lost or created execution", string(raw))
	}
}

func TestRemoteL4KeepsOnlyConfiguredProviderCredentialReference(t *testing.T) {
	config := `{"Remote":{"project_config":{"pr":{"token_env":"STEERLINE_GITEA_TOKEN"}}}}`
	result := strings.Join(e2eEnvironment([]string{"STEERLINE_GITEA_TOKEN=fixture-secret", "STEERLINE_OTHER_TOKEN=other", "STEERLINE_HOME=installation"}, config), "\n")
	if !strings.Contains(result, "STEERLINE_GITEA_TOKEN=fixture-secret") || strings.Contains(result, "STEERLINE_OTHER_TOKEN") || strings.Contains(result, "installation") {
		t.Fatal("provider reference dropped or unrelated project binding inherited")
	}
}

func TestRemoteL4RequiresLinkedSealedL2AndPR(t *testing.T) {
	report := env.Report{Version: env.ReportVersion, ActivityID: "activity", Round: 1, Commit: "fixture", ScriptDigest: strings.Repeat("a", 64), Completed: true, Checks: []env.Check{{ID: "tool", Status: "passed", Message: "available"}, {ID: "terminal.tty", Status: "passed", Message: "actual TTY"}}, L2: []env.Item{{ID: "l2.a", Status: "passed"}}, PR: env.PR{URL: "https://example.invalid/repo/pull/1", Base: "owned-base", Head: "owned-head", HeadRevision: strings.Repeat("b", 40), CleanupCompleted: true}}
	raw, _ := json.Marshal(report)
	encoded := base64.StdEncoding.EncodeToString(raw)
	sum := sha256.Sum256(raw)
	now := time.Now().UTC()
	result := runnercontract.ResultFile{Contract: runnercontract.ResultContract, RunID: "run", Version: 1, ObservedAt: &now, Encoding: "base64", ContentBase64: &encoded, SizeBytes: len(raw), SHA256: hex.EncodeToString(sum[:]), CollectionStatus: "closed"}
	for _, change := range []string{"valid", "wrong-source", "fourth-round", "unsealed", "missing-l2", "failed-l2", "unmerged-pr", "wrong-machine"} {
		t.Run(change, func(t *testing.T) {
			proof := map[string]any{"execution_digest": "digest", "source_commit": "fixture", "activity_id": "activity", "rounds": 1, "run_id": "run", "run": runnercontract.Run{RunID: "run", StopStatus: "stopped", Execution: runnercontract.Execution{Branch: "owned-head"}}, "result": result, "report": report, "l2_ids": []string{"l2.a"}}
			summary := map[string]any{"passed": true, "cleanup_complete": true, "level": "L2", "selected_ids": []string{"l2.a"}, "project_verification": []any{map[string]any{"test_id": "l2.a", "status": "passed"}}}
			pr := map[string]any{"merged": true, "merge_commit_sha": strings.Repeat("c", 40), "html_url": report.PR.URL, "head": map[string]string{"ref": "owned-head", "sha": report.PR.HeadRevision}, "base": map[string]string{"ref": "owned-base"}}
			provider := map[string]any{"pr": pr, "required_checks_verified": true, "required_reviews_verified": true, "execution_digest": "digest"}
			switch change {
			case "wrong-source":
				proof["source_commit"] = "other"
			case "fourth-round":
				proof["rounds"] = 4
			case "unsealed":
				r := result
				r.CollectionStatus = "pending"
				proof["result"] = r
			case "missing-l2":
				summary["selected_ids"] = []string{}
			case "failed-l2":
				summary["project_verification"] = []any{map[string]any{"test_id": "l2.a", "status": "failed"}}
			case "unmerged-pr":
				pr["merged"] = false
			case "wrong-machine":
				provider["execution_digest"] = "other"
			}
			files := map[string]any{"environment-proof.json": proof, "original-l2-summary.json": summary, "provider-pr-proof.json": provider}
			read := func(name string, out any) error {
				data, e := json.Marshal(files[name])
				if e != nil {
					return e
				}
				return json.Unmarshal(data, out)
			}
			err := checkSealedEnvironment(read, "digest", "fixture")
			if (err == nil) != (change == "valid") {
				t.Fatal(change, err)
			}
		})
	}
}

func TestRemoteL4AcceptsOnlyOriginalMixedUnreadyEvidence(t *testing.T) {
	execution := runnercontract.ExecutionInput{Adapter: "orca-terminal", Target: runnercontract.ExecutionTarget{Kind: "orca_environment", Environment: "desktop"}}
	digest := remotetest.Digest(execution)
	for _, change := range []string{"valid", "missing-round", "unsealed", "all-green", "pr-created", "wrong-machine", "duplicate-check"} {
		t.Run(change, func(t *testing.T) {
			details := []env.Round{}
			var last env.Report
			for i := 1; i <= 3; i++ {
				runID := "run-" + strings.Repeat("r", i)
				last = env.Report{Version: env.ReportVersion, ActivityID: "activity", Round: i, Commit: "fixture", ScriptDigest: strings.Repeat("a", 64), Completed: true, Checks: []env.Check{{ID: "git", Status: "passed", Message: "Git available"}, {ID: "unity.editor", Status: "failed", Message: "UNITY_EDITOR_PATH is not set"}}, L2: []env.Item{{ID: "l2.a", Status: "incomplete", Message: "requirements unmet"}}, PR: env.PR{Base: "owned-base", Head: "owned-head", CleanupCompleted: true}}
				if change == "all-green" {
					last.Checks[1].Status = "passed"
				}
				if change == "duplicate-check" {
					last.Checks[1].ID = "git"
				}
				raw, _ := json.Marshal(last)
				encoded := base64.StdEncoding.EncodeToString(raw)
				sum := sha256.Sum256(raw)
				now := time.Now().UTC()
				result := runnercontract.ResultFile{Contract: runnercontract.ResultContract, RunID: runID, Version: 1, ObservedAt: &now, Encoding: "base64", ContentBase64: &encoded, SizeBytes: len(raw), SHA256: hex.EncodeToString(sum[:]), CollectionStatus: "closed"}
				if change == "unsealed" {
					result.CollectionStatus = "pending"
				}
				input, _ := json.Marshal(runnercontract.RunInput{Execution: execution})
				details = append(details, env.Round{Number: i, Settled: true, RunID: runID, Run: &runnercontract.Run{RunID: runID, StopStatus: "stopped", Execution: runnercontract.Execution{Branch: "owned-head"}}, Result: &result, Request: runnercontract.Request{Input: input}})
			}
			if change == "missing-round" {
				details = details[:2]
			}
			proof := map[string]any{"execution_digest": digest, "source_commit": "fixture", "activity_id": "activity", "state": "failed", "rounds": 3, "round_details": details, "report": last, "l2_ids": []string{"l2.a"}}
			provider := map[string]any{"no_pr": change != "pr-created", "validation_base": "owned-base", "execution_digest": digest}
			if change == "wrong-machine" {
				provider["execution_digest"] = "wrong"
			}
			files := map[string]any{"environment-proof.json": proof, "provider-pr-proof.json": provider}
			read := func(name string, out any) error { raw, _ := json.Marshal(files[name]); return json.Unmarshal(raw, out) }
			if err := checkUnreadyEnvironment(read, digest, "fixture"); (err == nil) != (change == "valid") {
				t.Fatal(change, err)
			}
		})
	}
}
