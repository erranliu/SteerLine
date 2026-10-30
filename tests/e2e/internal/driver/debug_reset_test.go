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
