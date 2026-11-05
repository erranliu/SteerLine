//go:build linux

package billingconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRejectCaseOnlyLinkedHome(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "billing-home")
	if _, err := (Store{Home: real}).Save("*", draft()); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "BILLING-HOME")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Home: alias}).Read(); err == nil {
		t.Fatal("case-only linked home accepted")
	}
}
