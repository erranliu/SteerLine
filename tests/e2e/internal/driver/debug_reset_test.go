package driver

import (
	"os"
	"path/filepath"
	"testing"

	host "github.com/erranliu/SteerLine/internal/app/host"
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
