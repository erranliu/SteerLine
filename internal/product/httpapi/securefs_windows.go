//go:build windows

package httpapi

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const fileAllAccess = windows.ACCESS_MASK(0x1F01FF)

// createPrivateTemp attaches the protected, current-user-only DACL to the
// creation itself: preflight can inspect the file immediately after CreateFile.
func createPrivateTemp(dir, prefix string) (*os.File, error) {
	sid, err := currentUserSID()
	if err != nil {
		return nil, err
	}
	sd, err := windows.SecurityDescriptorFromString("O:" + sid.String() + "D:P(A;;FA;;;" + sid.String() + ")")
	if err != nil {
		return nil, err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	for attempts := 0; attempts < 100; attempts++ {
		suffix, err := randomHex(16)
		if err != nil {
			return nil, err
		}
		name := filepath.Join(dir, prefix+suffix+".tmp")
		path, err := windows.UTF16PtrFromString(name)
		if err != nil {
			return nil, err
		}
		handle, err := windows.CreateFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, &sa,
			windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
		runtime.KeepAlive(sd)
		if err == windows.ERROR_FILE_EXISTS || err == windows.ERROR_ALREADY_EXISTS {
			continue
		}
		if err != nil {
			return nil, &os.PathError{Op: "create", Path: name, Err: err}
		}
		return os.NewFile(uintptr(handle), name), nil
	}
	return nil, errors.New("unable to allocate a private local API temporary file")
}

func currentUserSID() (*windows.SID, error) {
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		return nil, err
	}
	return user.User.Sid, nil
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("local API directory is not a real directory")
	}
	return setAndVerifyPrivateACL(path)
}

func verifyPrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("local API directory is not a real directory")
	}
	sid, err := currentUserSID()
	if err != nil {
		return err
	}
	return verifyPrivateACL(path, sid)
}

func securePrivateFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("local API credential is not a regular file")
	}
	return setAndVerifyPrivateACL(path)
}

func verifyPrivateFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("local API credential is not a regular file")
	}
	sid, err := currentUserSID()
	if err != nil {
		return err
	}
	return verifyPrivateACL(path, sid)
}

func setAndVerifyPrivateACL(path string) error {
	sid, err := currentUserSID()
	if err != nil {
		return fmt.Errorf("read current Windows user SID: %w", err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + sid.String() + ")")
	if err != nil {
		return fmt.Errorf("build private Windows ACL: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	flags := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION | windows.OWNER_SECURITY_INFORMATION)
	if err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, flags, sid, nil, dacl, nil); err != nil {
		return fmt.Errorf("restrict local API ACL to current Windows user: %w", err)
	}
	return verifyPrivateACL(path, sid)
}

func verifyPrivateACL(path string, user *windows.SID) error {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("read local API Windows ACL: %w", err)
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !owner.Equals(user) {
		return fmt.Errorf("local API path is not owned by the current Windows user")
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return fmt.Errorf("local API Windows ACL inherits access from another directory")
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil || dacl.AceCount != 1 {
		return fmt.Errorf("local API Windows ACL does not have one current-user entry")
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err = windows.GetAce(dacl, 0, &ace); err != nil || ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || windows.ACCESS_MASK(ace.Mask) != fileAllAccess {
		return fmt.Errorf("local API Windows ACL does not grant the expected current-user access")
	}
	if ace.Header.AceFlags != 0 {
		return fmt.Errorf("local API Windows ACL contains inherited access")
	}
	aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !aceSID.IsValid() || !aceSID.Equals(user) {
		return fmt.Errorf("local API Windows ACL grants access to a different identity")
	}
	return nil
}

func privatePath(root, target string) bool {
	root, _ = filepath.Abs(root)
	target, _ = filepath.Abs(target)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || filepath.IsAbs(rel) {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
