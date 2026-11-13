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

func TestRemoteL4OnlyConsumesBoundSealedProjectResult(t *testing.T) {
	for _, change := range []string{"valid", "opaque-failed-detail", "wrong-source", "second-round", "unsealed", "wrong-machine", "wrong-inputs", "unverified-native", "wrong-input-digest", "wrong-run-projection", "missing-identity", "wrong-identity-source", "wrong-identity-context"} {
		t.Run(change, func(t *testing.T) {
			execution := runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"access_mode":"full_access"}`)}
			commit := strings.Repeat("a", 40)
			r := env.Record{ID: env.ID(), Project: "project", Execution: execution, Prepared: env.Prepared{Commit: commit, Inputs: strings.Repeat("b", 64)}}
			// Synthetic fixture only; actual live evidence freezes Runner's identity.
			r.Prepared.Identity = runnercontract.ExecutionIdentity{Contract: runnercontract.ExecutionIdentityContract, SourceExecutionDigest: runnercontract.ConfigurationValueDigest(execution), Context: execution}
			seed := env.InitialReport(r, 1)
			initial, _ := json.Marshal(seed)
			p := seed
			p.Completed = true
			p.Ready = true
			p.Message = "project ready"
			p.Details = json.RawMessage(`{"checks":[{"status":"failed"}],"project":"opaque"}`)
			if change == "wrong-inputs" {
				p.Inputs = "other"
			}
			body, _ := json.Marshal(p)
			encoded := base64.StdEncoding.EncodeToString(body)
			sum := sha256.Sum256(body)
			now := time.Now().UTC()
			result := runnercontract.ResultFile{Contract: runnercontract.ResultContract, RunID: "run", Version: 1, ObservedAt: &now, Encoding: "base64", ContentBase64: &encoded, SizeBytes: len(body), SHA256: hex.EncodeToString(sum[:]), CollectionStatus: "closed"}
			input := runnercontract.RunInput{Workflow: runnercontract.Workflow{ID: "execute", Version: 1}, Execution: execution, ResultFile: &runnercontract.ResultFileInput{InitialContentBase64: base64.StdEncoding.EncodeToString(initial)}}
			raw, _ := json.Marshal(input)
			run := runnercontract.Run{RunID: "run", Status: "completed", InputDigest: env.Digest(raw), StopStatus: "stopped", Correlation: map[string]string{"steerline_project": r.Project, "environment_activity": r.ID, "environment_round": "1"}, Final: &runnercontract.Final{Verified: true, NativeOutcome: "succeeded", CommandID: "command", SessionID: "session", TurnID: "turn"}}
			round := env.Round{Number: 1, Request: runnercontract.Request{Input: raw}, RunID: "run", InputDigest: run.InputDigest, Run: &run, Result: &result, Report: &p, Settled: true}
			proof := map[string]any{"execution_digest": remotetest.Digest(execution), "execution_identity": r.Prepared.Identity, "source_commit": commit, "activity_id": r.ID, "rounds": 1, "round_details": []env.Round{round}, "run_id": "run", "run": run, "result": result, "report": p}
			switch change {
			case "missing-identity":
				delete(proof, "execution_identity")
			case "wrong-identity-source":
				identity := r.Prepared.Identity
				identity.SourceExecutionDigest = strings.Repeat("c", 64)
				proof["execution_identity"] = identity
			case "wrong-identity-context":
				identity := r.Prepared.Identity
				identity.Context.Settings = json.RawMessage(`{"fixture":"different"}`)
				proof["execution_identity"] = identity
			case "wrong-source":
				proof["source_commit"] = "other"
			case "second-round":
				proof["rounds"] = 2
			case "unsealed":
				result.CollectionStatus = "pending"
				proof["result"] = result
			case "wrong-machine":
				proof["execution_digest"] = "different"
			case "wrong-input-digest":
				round.InputDigest = "different"
				proof["round_details"] = []env.Round{round}
			case "wrong-run-projection":
				copy := run
				copy.Status = "failed"
				proof["run"] = copy
			case "unverified-native":
				run.Final.Verified = false
				proof["run"] = run
			}
			read := func(name string, target any) error {
				if name != "environment-proof.json" {
					t.Fatal("business evidence was rechecked", name)
				}
				b, _ := json.MarshalIndent(proof, "", "  ")
				return json.Unmarshal(b, target)
			}
			err := checkSealedEnvironment(read, remotetest.Digest(execution), commit)
			if (err == nil) != (change == "valid" || change == "opaque-failed-detail") {
				t.Fatal(change, err)
			}
		})
	}
}
