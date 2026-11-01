package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/localservice"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
)

func TestRunnerUpgradePreservesOldNamespace(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "application-api")
	pin := runnercontract.BuildPin{StateDir: filepath.Join(home, "original-v6"), Executable: filepath.Join(home, "original.exe"), SHA256: strings.Repeat("a", 64)}
	require(t, localservice.Write(dir, "runner-build.json", pin))
	before, err := os.ReadFile(filepath.Join(dir, "runner-build.json"))
	require(t, err)
	_, err = VerifyRunnerUpgrade(context.Background(), home, "unavailable-candidate", "bad")
	if err == nil || !strings.Contains(err.Error(), "fresh InstallHome") {
		t.Fatal("old namespace was not fenced before invoking candidate", err)
	}
	after, err := os.ReadFile(filepath.Join(dir, "runner-build.json"))
	require(t, err)
	if !bytes.Equal(before, after) {
		t.Fatal("preflight rewrote original binding")
	}
	if _, err := os.Stat(runnerstatepath.Directory(home)); !os.IsNotExist(err) {
		t.Fatal("preflight created new state", err)
	}
}

func TestRunnerUpgradeDrainedBusinessAndResources(t *testing.T) {
	for _, fault := range []string{"valid", "closed", "closed-failure", "open-failure", "accepted", "confirmed", "rejected", "pending", "in_flight", "outcome_unknown", "unknown-state", "service", "resources", "business", "input", "manual", "resume"} {
		t.Run(fault, func(t *testing.T) {
			cli := startupFixtureCLI(t)
			run := runnercontract.Run{RunID: "original", Revision: 3, InputDigest: "fixed", Status: "completed", StopStatus: "stopped"}
			switch fault {
			case "closed":
				run.Status, run.Closed = "closed", true
			case "closed-failure":
				run.Status, run.Closed = "failed", true
			case "open-failure":
				run.Status = "failed"
			case "accepted", "confirmed", "rejected", "pending", "in_flight", "outcome_unknown", "unknown-state":
				run.Operations = []runnercontract.Operation{{ID: "original-op", Action: "start_turn", Status: fault}}
			case "service":
				require(t, os.WriteFile(filepath.Join(cli.StateDir, "idle-fixture.json"), mustJSON(runnerclient.ServiceInfo{Protocol: runnerclient.Protocol, StateDir: cli.StateDir, Status: "ready"}), 0600))
			case "resources":
				run.StopStatus = "running"
			case "business":
				run.Status = "pr_ready"
			case "input":
				run.NewInputsAllowed = true
			case "manual":
				run.Phase, run.ResumeRequired = "manual_intervention_required", true
			case "resume":
				run.Controls.CanResume = true
			}
			writeStartupInspection(t, cli, run, "submit")
			require(t, os.WriteFile(filepath.Join(cli.StateDir, "caller-fixture.json"), mustJSON(map[string]any{"runs": map[string]any{run.RunID: run}}), 0600))
			before, err := os.ReadFile(filepath.Join(cli.StateDir, "caller-fixture.json"))
			require(t, err)
			err = cli.VerifyDrained(context.Background())
			valid := fault == "valid" || fault == "closed" || fault == "closed-failure" || fault == "accepted" || fault == "confirmed" || fault == "rejected"
			if (err == nil) != valid {
				t.Fatal("upgrade drainage admission differs", fault, err)
			}
			after, err := os.ReadFile(filepath.Join(cli.StateDir, "caller-fixture.json"))
			require(t, err)
			if !bytes.Equal(before, after) {
				t.Fatal("drainage query changed original facts")
			}
		})
	}
}
