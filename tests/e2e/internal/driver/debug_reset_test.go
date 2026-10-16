package driver

import (
	"os"
	"path/filepath"
	"testing"

	host "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
)

func TestDebugResetBeforeInitializationKeepsFreshPackageAndHonorsHostLock(t *testing.T) {
	root := t.TempDir()
	d := &debugRelease{root: root, home: filepath.Join(root, "home"), repo: filepath.Join(root, "repo")}
	api := filepath.Join(d.home, "application-api")
	for _, dir := range []string{filepath.Join(d.repo, ".git"), api} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
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
	r := runnerstate.EnvironmentCheckRecord{Operations: []runnerstate.EnvironmentCheckOperation{{Status: "outcome_unknown"}}}
	if environmentCheckResetSafety(r) == nil {
		t.Fatal("unknown start erased")
	}
	r.Operations[0].Status = "applied"
	r.Report = &runnercontract.Check{Resources: runnercontract.CheckResources{Status: "pending"}}
	if environmentCheckResetSafety(r) == nil {
		t.Fatal("active resources erased")
	}
	r.Report.Resources.Status = "settled"
	r.Operations = append(r.Operations, runnerstate.EnvironmentCheckOperation{Status: "outcome_unknown"})
	if environmentCheckResetSafety(r) == nil {
		t.Fatal("uncertain stop erased using older settled report")
	}
	r.Operations[1].Status = "applied"
	if err := environmentCheckResetSafety(r); err != nil {
		t.Fatal(err)
	}
	r.Report = nil
	r.Operations = r.Operations[:1]
	r.Operations[0].Status = "rejected"
	if err := environmentCheckResetSafety(r); err != nil {
		t.Fatal("definitively rejected start blocked reset", err)
	}
	if len(debugHistory) < 2 || debugHistory[0] != "environment_check_requests" || debugHistory[1] != "environment_checks" {
		t.Fatal("check request associations not reset before their owner")
	}
}
