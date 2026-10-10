//go:build windows

package httpapi

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestAPIPrivateTemporaryACLRejectsUnsafeEntry(t *testing.T) {
	runDir := t.TempDir()
	root := filepath.Join(runDir, "local-api")
	commands := filepath.Join(root, "commands")
	if err := ensurePrivateDirectory(root); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateDirectory(commands); err != nil {
		t.Fatal(err)
	}
	unsafe, err := os.CreateTemp(commands, ".credential-*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	if err := unsafe.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ValidateStateDirectory(runDir, "C:/repo"); err == nil {
		t.Fatal("inherited temporary-file DACL was ignored")
	}
	if err := os.Remove(unsafe.Name()); err != nil {
		t.Fatal(err)
	}
	file, err := createPrivateTemp(commands, ".credential-")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ValidateStateDirectory(runDir, "C:/repo"); err != nil {
		t.Fatalf("creation-time private ACL did not pass: %v", err)
	}
	sid, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + sid.String() + ")(A;;GR;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(file.Name(), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	if err := ValidateStateDirectory(runDir, "C:/repo"); err == nil {
		t.Fatal("temporary file with unauthorized reader passed preflight")
	}
}

func TestAPIPrivateDirectoryOwnedWithModifyAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "normal-owned-directory")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	sid, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;0x1301bf;;;" + sid.String() + ")")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateDirectory(path); err != nil {
		t.Fatalf("current owner needed WRITE_OWNER to secure its DACL: %v", err)
	}
	if err := verifyPrivateDirectory(path); err != nil {
		t.Fatal(err)
	}
}
