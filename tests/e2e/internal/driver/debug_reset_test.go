package driver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	host "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
)

func TestDebugResetBeforeInitializationKeepsFreshPackageAndHonorsHostLock(t *testing.T) {
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-fixture")
	root := t.TempDir()
	d := &debugRelease{root: root, home: filepath.Join(root, "home"), repo: filepath.Join(root, "repo")}
	api := filepath.Join(d.home, "application-api")
	for _, dir := range []string{filepath.Join(d.repo, ".git"), api} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	raw, err := json.Marshal(map[string]any{"runner": runnerclient.Selection{Executable: exe, SHA256: hex.EncodeToString(sum[:]), Version: "unit-fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d.home, "execution.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	lock, err := host.LockRepository(api)
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []bool{true, false} {
		if err := d.reset(check); err == nil {
			t.Fatal("fresh database state bypassed active host lock")
		}
	}
	lock.Close()
	for _, check := range []bool{true, false} {
		if err := d.reset(check); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(d.stateDir()); !os.IsNotExist(err) {
			t.Fatalf("reset initialized fresh state: %v", err)
		}
	}
}

func TestEnvironmentCheckDebugResetPreservesUncertainResources(t *testing.T) {
	r := runnerstate.EnvironmentCheckRecord{StateDir: "original-directory", CheckID: "original", Operations: []runnerstate.EnvironmentCheckOperation{{Status: "outcome_unknown"}}}
	proof := map[string]runnerclient.StoppedCheck{"original": {CheckID: "original", InputDigest: "digest", ResourcesSettled: true}}
	verify := func() error { return environmentCheckResetSafety(r, "original-directory", proof) }
	if verify() == nil {
		t.Fatal("unknown start erased")
	}
	r.Operations[0].Status = "applied"
	r.Report = &runnercontract.Check{InputDigest: "digest", CleanupStatus: "pending"}
	if environmentCheckResetSafety(r, "original-directory", nil) == nil {
		t.Fatal("active resources erased without shared proof")
	}
	if err := verify(); err != nil {
		t.Fatal("cached pre-shutdown report overrode final shared proof", err)
	}
	r.Operations = append(r.Operations, runnerstate.EnvironmentCheckOperation{Status: "outcome_unknown"})
	if verify() == nil {
		t.Fatal("uncertain stop erased using older settled report")
	}
	r.Operations[1].Status = "applied"
	if err := verify(); err != nil {
		t.Fatal(err)
	}
	for _, alter := range []func(*runnerstate.EnvironmentCheckRecord){
		func(v *runnerstate.EnvironmentCheckRecord) { v.StateDir = "other-directory" },
		func(v *runnerstate.EnvironmentCheckRecord) { v.CheckID = "other-check" },
		func(v *runnerstate.EnvironmentCheckRecord) {
			v.Report = &runnercontract.Check{InputDigest: "other-digest"}
		},
	} {
		bad := r
		alter(&bad)
		if environmentCheckResetSafety(bad, "original-directory", proof) == nil {
			t.Fatal("changed resource binding accepted")
		}
	}
	r.Report = nil
	r.CheckID = ""
	r.Operations = r.Operations[:1]
	r.Operations[0].Status = "rejected"
	if err := verify(); err != nil {
		t.Fatal("definitively rejected start blocked reset", err)
	}
	if len(debugHistory) < 2 || debugHistory[0] != "environment_check_requests" || debugHistory[1] != "environment_checks" {
		t.Fatal("check request associations not reset before their owner")
	}
}
