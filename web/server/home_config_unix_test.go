//go:build !windows

package server

import (
	"os"
	"testing"
)

func homePermissionsFixture(t *testing.T, home string) func() {
	t.Helper()
	if err := os.Chmod(home, 0750); err != nil {
		t.Fatal(err)
	}
	return func() {
		t.Helper()
		info, err := os.Stat(home)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0750 {
			t.Errorf("home permissions changed while creating Web config: %v", info.Mode())
		}
	}
}
