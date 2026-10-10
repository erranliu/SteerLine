//go:build windows

package pathguard

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestPersistentSQLiteSidecarACLIsNotIgnored(t *testing.T) {
	database := filepath.Join(t.TempDir(), "state.db")
	shm := database + "-shm"
	if err := os.WriteFile(shm, []byte("sidecar"), 0600); err != nil {
		t.Fatal(err)
	}
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(D;;FA;;;WD)(A;;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	flags := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION)
	if err = windows.SetNamedSecurityInfo(shm, windows.SE_FILE_OBJECT, flags, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(shm)
	if err != nil {
		t.Fatal(err)
	}
	handle, directErr := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if directErr == nil {
		_ = windows.CloseHandle(handle)
		t.Fatal("ACL fixture still grants FILE_READ_ATTRIBUTES")
	}
	if !errors.Is(directErr, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("ACL fixture returned an unexpected direct-open error: %v", directErr)
	}
	if err = RejectSharedSQLiteFiles(database); !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("persistent sidecar ACL denial was ignored or obscured: %v", err)
	}
}

func TestTransientSQLiteSidecarAccessDeniedIsRecheckedAsMissing(t *testing.T) {
	database := filepath.Join(t.TempDir(), "state.db")
	shm := database + "-shm"
	if err := os.WriteFile(shm, []byte("sidecar"), 0600); err != nil {
		t.Fatal(err)
	}
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sid := user.User.Sid.String()
	sd, err := windows.SecurityDescriptorFromString("D:P(D;;FR;;;" + sid + ")(A;;FA;;;" + sid + ")")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	flags := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION)
	if err = windows.SetNamedSecurityInfo(shm, windows.SE_FILE_OBJECT, flags, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(shm)
	if err != nil {
		t.Fatal(err)
	}
	if handle, openErr := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0); openErr == nil {
		_ = windows.CloseHandle(handle)
		t.Fatal("transient-denial fixture still grants file reads")
	} else if !errors.Is(openErr, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("transient-denial fixture returned an unexpected error: %v", openErr)
	}
	removed := make(chan error, 1)
	timer := time.AfterFunc(5*time.Millisecond, func() { removed <- os.Remove(shm) })
	links, countErr := fileLinkCount(shm)
	if timer.Stop() {
		removed <- os.Remove(shm)
	}
	if removeErr := <-removed; removeErr != nil && !os.IsNotExist(removeErr) {
		t.Fatalf("remove the transient sidecar after the denied inspection: %v", removeErr)
	}
	if countErr != nil || links != 0 {
		t.Fatalf("transiently inaccessible, removed SQLite sidecar did not recheck as missing: links=%d err=%v", links, countErr)
	}
}
