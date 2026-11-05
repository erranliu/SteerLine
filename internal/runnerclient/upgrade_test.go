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
	encode := func(v any) string {
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		return string(b)
	}
	for _, fault := range []string{"valid", "original-active", "candidate-active", "original-input", "candidate-input", "original-unknown", "candidate-unknown", "service-running", "unsettled", "foreign-state", "other-state",
		"historical-pid", "historical-pid-missing-shutdown", "historical-pid-wrong-build", "historical-pid-unsettled-shutdown", "historical-pid-original-no-stop", "historical-pid-candidate-no-stop", "historical-pid-active-process", "historical-pid-foreign-scope", "historical-pid-duplicate", "historical-pid-missing-input"} {
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
			service := "not_running"
			if fault == "service-running" {
				service = "ready"
			}
			info := ServiceInfo{Protocol: Protocol, StateDir: dir, Status: service}
			proof := StoppedVerification{StateDir: dir, ResourcesSettled: true, Runs: []StoppedRun{{RunID: "run", InputDigest: "fixed", ResourcesSettled: true, StopEvidenceScope: "orca_terminal"}}}
			if fault == "unsettled" {
				proof.ResourcesSettled, proof.Runs[0].ResourcesSettled = false, false
				proof.Runs[0].Diagnostic = "original Orca terminal is still active"
			}
			if fault == "foreign-state" {
				proof.StateDir = filepath.Join(dir, "foreign")
			}
			if strings.HasPrefix(fault, "historical-pid") {
				info.LastShutdown = &ServiceShutdown{StateDir: dir, InstanceID: "old", RequestID: "sl-service-stop-old", ExecutableSHA256: original.SHA256, ResourcesSettled: true}
				proof.ResourcesSettled = false
				proof.Runs[0] = StoppedRun{RunID: a.RunID, InputDigest: a.InputDigest, StopEvidenceScope: "orca_local_codex_resources",
					Diagnostic: "run run: original native writer absence is unverified: original local process 26628 is unknown: Access is denied."}
				a.StopEvidenceScope, b.StopEvidenceScope = proof.Runs[0].StopEvidenceScope, proof.Runs[0].StopEvidenceScope
				a.Operations = []runnercontract.Operation{{ID: "stop", Action: "stop_execution", Status: "confirmed"}}
				b.Operations = a.Operations
				switch fault {
				case "historical-pid-missing-shutdown":
					info.LastShutdown = nil
				case "historical-pid-wrong-build":
					info.LastShutdown.ExecutableSHA256 = strings.Repeat("0", 64)
				case "historical-pid-unsettled-shutdown":
					info.LastShutdown.ResourcesSettled = false
				case "historical-pid-original-no-stop":
					a.Operations = nil
				case "historical-pid-candidate-no-stop":
					b.Operations = nil
				case "historical-pid-active-process":
					proof.Runs[0].Diagnostic = strings.Replace(proof.Runs[0].Diagnostic, "is unknown", "is matches", 1)
				case "historical-pid-foreign-scope":
					proof.Runs[0].StopEvidenceScope = "orca_terminal"
				case "historical-pid-duplicate":
					proof.Runs = append(proof.Runs, proof.Runs[0])
				case "historical-pid-missing-input":
					proof.Runs[0].InputDigest = ""
				}
			}
			t.Setenv("STEERLINE_UPGRADE_ORIGINAL_RUN", encode(Inspection{Run: &a}))
			t.Setenv("STEERLINE_UPGRADE_CANDIDATE_RUN", encode(Inspection{Run: &b}))
			t.Setenv("STEERLINE_UPGRADE_SERVICE", encode(info))
			t.Setenv("STEERLINE_UPGRADE_PROOF", encode(proof))
			selected := candidate
			if fault == "other-state" {
				selected.StateDir = filepath.Join(dir, "other")
			}
			err := original.VerifyUpgradeDrained(context.Background(), selected)
			if (err == nil) != (fault == "valid" || fault == "historical-pid") {
				t.Fatal("upgrade admission differs", err)
			}
			log, _ := os.ReadFile(calls)
			if strings.Contains(string(log), "original"+filepath.Ext(self)+" service verify-stopped") || strings.Contains(string(log), "service start") {
				t.Fatal("upgrade invoked old native verifier or started a writer", string(log))
			}
			if fault == "valid" && (!strings.Contains(string(log), "original"+filepath.Ext(self)+" inspect") || !strings.Contains(string(log), "candidate"+filepath.Ext(self)+" inspect")) {
				t.Fatal("missing independent business reader", string(log))
			}
			if fault == "historical-pid" {
				oldSHA := strings.Repeat("a", 64)
				info.LastShutdown.ExecutableSHA256 = oldSHA
				t.Setenv("STEERLINE_UPGRADE_SERVICE", encode(info))
				if err := candidate.VerifyInstalledUpgradeDrained(context.Background(), oldSHA); err != nil {
					t.Fatal("installed candidate rejected original shutdown", err)
				}
				if err := candidate.VerifyInstalledUpgradeDrained(context.Background(), candidate.SHA256); err == nil {
					t.Fatal("installed candidate accepted a shutdown from another build")
				}
				if err := candidate.VerifyDrained(context.Background()); err == nil {
					t.Fatal("unbound drainage accepted the original shutdown")
				}
			}
		})
	}
}
