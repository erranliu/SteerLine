package remoteprotocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureCommit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func verifyExpectation() VerifyExpectation {
	return VerifyExpectation{
		ProjectID:       "project-1",
		TaskID:          "business-task-1",
		WorkflowRunID:   "workflow-1",
		ExecutionID:     "execution-1",
		AttemptID:       "attempt-1",
		OperationID:     "operation-1",
		OrcaTaskID:      "task-test-1",
		OrcaDispatchID:  "ctx-test-1",
		InputRevision:   "implement-result-1",
		WorktreeID:      "remote-worktree-1",
		WorktreeBranch:  "codex/test",
		CandidateCommit: fixtureCommit,
		ObservedHead:    fixtureCommit,
		RequiredChecks:  []string{"unit", "lint"},
	}
}

func readVerifyFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestValidateVerifyGate(t *testing.T) {
	done := WorkerDone{TaskID: "task-test-1", DispatchID: "ctx-test-1", Outcome: "succeeded"}
	for _, test := range []struct {
		name    string
		fixture string
		wantErr string
	}{
		{"valid", "verify-valid.json", ""},
		{"missing required receipt despite success summary", "verify-missing-check.json", `required check "lint" missing`},
		{"nonzero exit despite success summary", "verify-nonzero.json", `check "unit" did not pass`},
		{"wrong candidate despite success summary", "verify-wrong-commit.json", "candidate_commit mismatch"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateVerifyGate(readVerifyFixture(t, test.fixture), done, verifyExpectation())
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("valid result rejected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("want error containing %q, got %v", test.wantErr, err)
			}
		})
	}
}

func TestValidateVerifyGateBindsHostAndSettlement(t *testing.T) {
	valid := readVerifyFixture(t, "verify-valid.json")
	done := WorkerDone{TaskID: "task-test-1", DispatchID: "ctx-test-1", Outcome: "succeeded"}
	for _, test := range []struct {
		name    string
		change  func(*WorkerDone, *VerifyExpectation)
		wantErr string
	}{
		{"wrong dispatch", func(d *WorkerDone, _ *VerifyExpectation) { d.DispatchID = "ctx-other" }, "worker_done identity mismatch"},
		{"failed worker settlement", func(d *WorkerDone, _ *VerifyExpectation) { d.Outcome = "failed" }, "does not pass verify"},
		{"host HEAD changed", func(_ *WorkerDone, w *VerifyExpectation) { w.ObservedHead = strings.Repeat("b", 40) }, "observed worktree HEAD differs"},
		{"missing expected checks", func(_ *WorkerDone, w *VerifyExpectation) { w.RequiredChecks = nil }, "lacks required checks"},
	} {
		t.Run(test.name, func(t *testing.T) {
			gotDone, want := done, verifyExpectation()
			test.change(&gotDone, &want)
			err := ValidateVerifyGate(valid, gotDone, want)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("want error containing %q, got %v", test.wantErr, err)
			}
		})
	}
}

func TestValidateVerifyGateRequiresExplicitFields(t *testing.T) {
	var result map[string]any
	if err := json.Unmarshal(readVerifyFixture(t, "verify-valid.json"), &result); err != nil {
		t.Fatal(err)
	}
	delete(result, "error")
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	done := WorkerDone{TaskID: "task-test-1", DispatchID: "ctx-test-1", Outcome: "succeeded"}
	if err := ValidateVerifyGate(data, done, verifyExpectation()); err == nil || !strings.Contains(err.Error(), "lacks error") {
		t.Fatalf("missing error field was accepted: %v", err)
	}
	result["error"] = nil
	result["future_field"] = "ignored"
	data, err = json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateVerifyGate(data, done, verifyExpectation()); err != nil {
		t.Fatalf("compatible unknown field was rejected: %v", err)
	}
}

func TestValidateVerifyGateRejectsInvalidCheckReceipt(t *testing.T) {
	done := WorkerDone{TaskID: "task-test-1", DispatchID: "ctx-test-1", Outcome: "succeeded"}
	for _, test := range []struct {
		name    string
		change  func(map[string]any)
		wantErr string
	}{
		{"missing exit code", func(check map[string]any) { delete(check, "exit_code") }, `check "unit" did not pass`},
		{"check ran on another commit", func(check map[string]any) { check["actual_commit"] = strings.Repeat("b", 40) }, `check "unit" commit mismatch`},
		{"missing evidence", func(check map[string]any) { check["evidence_ref"] = "" }, `check "unit" lacks command, script version or evidence`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var result map[string]any
			if err := json.Unmarshal(readVerifyFixture(t, "verify-valid.json"), &result); err != nil {
				t.Fatal(err)
			}
			checks := result["checks"].([]any)
			test.change(checks[0].(map[string]any))
			data, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			err = ValidateVerifyGate(data, done, verifyExpectation())
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("want error containing %q, got %v", test.wantErr, err)
			}
		})
	}
}
