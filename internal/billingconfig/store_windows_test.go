//go:build windows

package billingconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/erranliu/SteerLine/internal/product/httpfs"
	"golang.org/x/sys/windows"
)

func TestBillingPublicationPreservesHomeAccess(t *testing.T) {
	home := t.TempDir()
	sid, err := httpfs.CurrentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + sid.String() + ")")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	setHomeACL := func() {
		t.Helper()
		if err := windows.SetNamedSecurityInfo(home, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
			t.Fatal(err)
		}
	}
	setHomeACL()
	// Restore inheritable access before TempDir cleanup, even with the old bug.
	t.Cleanup(setHomeACL)
	paths := []string{home}
	for _, name := range []string{"bin/steerline.exe", "bin/steerline-web.exe", "bin/steerrunner.exe", "etc/project.json", "state/project/steerline.db"} {
		path := filepath.Join(home, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, filepath.Dir(path), path)
	}
	private := filepath.Join(home, "application-api")
	if err := httpfs.EnsurePrivateDirectory(private); err != nil {
		t.Fatal(err)
	}
	paths = append(paths, private)
	security := func(path string) string {
		t.Helper()
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		return sd.String()
	}
	before := map[string]string{}
	for _, path := range paths {
		before[path] = security(path)
	}
	store := Store{Home: home}
	one, err := store.Save("*", draft())
	if err != nil {
		t.Fatal(err)
	}
	two, err := store.Save(one.Digest, draft())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Activate(two.Digest, one.Active); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if after := security(path); after != before[path] {
			t.Errorf("billing publication changed ACL on %s: before=%s after=%s", path, before[path], after)
		}
	}
	for _, name := range []string{"bin/steerline.exe", "bin/steerline-web.exe", "bin/steerrunner.exe", "etc/project.json", "state/project/steerline.db"} {
		if data, err := os.ReadFile(filepath.Join(home, filepath.FromSlash(name))); err != nil || string(data) != name {
			t.Errorf("billing publication damaged access/content on %s: %q %v", name, data, err)
		}
	}
	if err := httpfs.VerifyPrivateFile(filepath.Join(home, "billing.json")); err != nil {
		t.Fatal("billing file lost private ACL", err)
	}
}
