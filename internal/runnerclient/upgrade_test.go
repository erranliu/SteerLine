package runnerclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestUpgradeDrainedUsesCandidateProofAndBothBusinessReaders(t *testing.T) {
	dir := t.TempDir()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	makeClient := func(name string) Client {
		exe := filepath.Join(dir, name+filepath.Ext(self))
		if err := os.WriteFile(exe, raw, 0700); err != nil {
			t.Fatal(err)
		}
		return Client{Executable: exe, StateDir: dir, SHA256: hex.EncodeToString(sum[:])}
	}
	original, candidate := makeClient("original"), makeClient("candidate")
	t.Setenv("STEERLINE_RUNNER_CLI_FIXTURE", "upgrade-drain")
	t.Setenv("STEERLINE_RUNNER_CLI_CALL", filepath.Join(dir, "last-call"))
	t.Setenv("STEERLINE_UPGRADE_DIR", dir)
	encode := func(v any) string {
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		return string(b)
	}
	for _, fault := range []string{"valid", "original-active", "candidate-active", "original-input", "candidate-input", "original-unknown", "candidate-unknown", "service-running", "unsettled", "foreign-state", "other-state"} {
		t.Run(fault, func(t *testing.T) {
			calls := filepath.Join(dir, fault+".calls")
			t.Setenv("STEERLINE_UPGRADE_CALLS", calls)
			a := Run{RunID: "run", Revision: 1, InputDigest: "fixed", Status: "closed", Closed: true, StopStatus: "stopped"}
			b := a
			alter := func(r *Run) {
				switch {
				case strings.HasSuffix(fault, "active"):
					r.Status, r.Closed = "pr_ready", false
				case strings.HasSuffix(fault, "input"):
					r.InputDigest = "different"
				case strings.HasSuffix(fault, "unknown"):
					r.Operations = []runnercontract.Operation{{ID: "stop", Action: "stop_execution", Status: "outcome_unknown"}}
				}
			}
			if strings.HasPrefix(fault, "original-") {
				alter(&a)
			}
			if strings.HasPrefix(fault, "candidate-") {
				alter(&b)
			}
			t.Setenv("STEERLINE_UPGRADE_ORIGINAL_RUN", encode(Inspection{Run: &a}))
			t.Setenv("STEERLINE_UPGRADE_CANDIDATE_RUN", encode(Inspection{Run: &b}))
			service := "not_running"
			if fault == "service-running" {
				service = "ready"
			}
			t.Setenv("STEERLINE_UPGRADE_SERVICE", service)
			proof := StoppedVerification{StateDir: dir, ResourcesSettled: true, Runs: []StoppedRun{{RunID: "run", InputDigest: "fixed", ResourcesSettled: true, StopEvidenceScope: "orca_terminal"}}}
			if fault == "unsettled" {
				proof.ResourcesSettled, proof.Runs[0].ResourcesSettled = false, false
			}
			if fault == "foreign-state" {
				proof.StateDir = filepath.Join(dir, "foreign")
			}
			t.Setenv("STEERLINE_UPGRADE_PROOF", encode(proof))
			selected := candidate
			if fault == "other-state" {
				selected.StateDir = filepath.Join(dir, "other")
			}
			err := original.VerifyUpgradeDrained(context.Background(), selected)
			if (err == nil) != (fault == "valid") {
				t.Fatal("upgrade admission differs", err)
			}
			log, _ := os.ReadFile(calls)
			if strings.Contains(string(log), "original"+filepath.Ext(self)+" service verify-stopped") || strings.Contains(string(log), "service start") {
				t.Fatal("upgrade invoked old native verifier or started a writer", string(log))
			}
			if fault == "valid" && (!strings.Contains(string(log), "original"+filepath.Ext(self)+" inspect") || !strings.Contains(string(log), "candidate"+filepath.Ext(self)+" inspect")) {
				t.Fatal("missing independent business reader", string(log))
			}
		})
	}
}
