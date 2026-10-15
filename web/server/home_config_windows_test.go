//go:build windows

package server

import (
	"testing"

	privatefs "github.com/erranliu/SteerLine/internal/product/httpfs"
	"golang.org/x/sys/windows"
)

func homePermissionsFixture(t *testing.T, home string) func() {
	t.Helper()
	sid, err := privatefs.CurrentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	// Model an installation home whose launchers inherit access from the root.
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + sid.String() + ")")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	apply := func() {
		t.Helper()
		if err := windows.SetNamedSecurityInfo(home, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
			t.Fatal(err)
		}
	}
	apply()
	// Even a failing regression must restore inherited access for temp cleanup.
	t.Cleanup(apply)
	read := func() string {
		t.Helper()
		sd, err := windows.GetNamedSecurityInfo(home, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		return sd.String()
	}
	before := read()
	return func() {
		t.Helper()
		if after := read(); after != before {
			t.Errorf("home ACL changed while creating Web config: before=%s after=%s", before, after)
		}
	}
}
