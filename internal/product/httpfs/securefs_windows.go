//go:build windows

package httpfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const fileAllAccess = windows.ACCESS_MASK(0x1F01FF)

// Replace using Windows' snapshot-preserving rename semantics. MoveFileEx (as
// used by os.Rename) still excludes an open destination even when its readers
// share deletion. Persistent sharing/permission failures remain bounded errors.
func ReplacePrivateFile(source, target string) error {
	var err error
	for attempt := 0; attempt < 40; attempt++ {
		err = renamePrivateSnapshot(source, target)
		if err == nil || (!errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_ACCESS_DENIED)) {
			return err
		}
		if attempt < 39 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	return err
}

type privateRenameInfo struct {
	flags         uint32
	rootDirectory windows.Handle
	fileNameBytes uint32
	fileName      [1]uint16
}

func renamePrivateSnapshot(source, target string) error {
	rename := func() error {
		if source == "" || target == "" {
			return os.ErrInvalid
		}
		from, err := windows.UTF16PtrFromString(source)
		if err != nil {
			return err
		}
		handle, err := windows.CreateFile(from, windows.DELETE|windows.SYNCHRONIZE,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
			nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err != nil {
			return fmt.Errorf("open rename source: %w", err)
		}
		defer windows.CloseHandle(handle)
		var file windows.ByHandleFileInformation
		if err = windows.GetFileInformationByHandle(handle, &file); err != nil {
			return fmt.Errorf("inspect rename source: %w", err)
		}
		if file.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
			return errors.New("linked/nonregular private rename source")
		}
		absolute, err := filepath.Abs(target)
		if err != nil {
			return err
		}
		name, err := windows.UTF16FromString(absolute)
		if err != nil {
			return err
		}
		var layout privateRenameInfo
		offset := unsafe.Offsetof(layout.fileName)
		nameBytes := (len(name) - 1) * 2
		// The Win32 FILE_RENAME_INFO name is NUL-terminated; its byte count
		// excludes that terminator. Keep both representations complete.
		buffer := make([]byte, int(offset)+len(name)*2)
		info := (*privateRenameInfo)(unsafe.Pointer(&buffer[0]))
		info.flags = windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS
		info.fileNameBytes = uint32(nameBytes)
		copy(unsafe.Slice(&info.fileName[0], len(name)), name)
		if err := windows.SetFileInformationByHandle(handle, windows.FileRenameInfoEx, &buffer[0], uint32(len(buffer))); err != nil {
			return fmt.Errorf("publish rename target (FileRenameInfoEx): %w", err)
		}
		return nil
	}
	if err := rename(); err != nil {
		return &os.LinkError{Op: "rename", Old: source, New: target, Err: err}
	}
	return nil
}

// Discovery is atomically replaced while clients observe startup/ready state.
// Hold a snapshot handle without excluding the writer's rename or deletion.
func openDiscoveryReader(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(handle, &info); err != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		windows.CloseHandle(handle)
		if err == nil {
			err = errors.New("linked/nonregular discovery file")
		}
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}

// CreatePrivateTemp attaches the protected, current-user-only DACL to the
// creation itself: preflight can inspect the file immediately after CreateFile.
func CreatePrivateTemp(dir, prefix string) (*os.File, error) {
	sid, err := CurrentUserSID()
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

func CurrentUserSID() (*windows.SID, error) {
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		return nil, err
	}
	return user.User.Sid, nil
}

func EnsurePrivateDirectory(path string) error {
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

func VerifyPrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("local API directory is not a real directory")
	}
	sid, err := CurrentUserSID()
	if err != nil {
		return err
	}
	return verifyPrivateACL(path, sid)
}

func SecurePrivateFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("local API credential is not a regular file")
	}
	return setAndVerifyPrivateACL(path)
}

func VerifyPrivateFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("local API credential is not a regular file")
	}
	sid, err := CurrentUserSID()
	if err != nil {
		return err
	}
	return verifyPrivateACL(path, sid)
}

func setAndVerifyPrivateACL(path string) error {
	sid, err := CurrentUserSID()
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
	current, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("read local API owner before securing ACL: %w", err)
	}
	owner, _, err := current.Owner()
	if err != nil {
		return err
	}
	flags := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION)
	var nextOwner *windows.SID
	if owner == nil || !owner.Equals(sid) {
		flags |= windows.OWNER_SECURITY_INFORMATION
		nextOwner = sid
	}
	// An existing owner has implicit WRITE_DAC, not WRITE_OWNER. Do not
	// demand ownership reassignment on a normal Modify-access directory.
	if err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, flags, nextOwner, nil, dacl, nil); err != nil {
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

func PrivatePath(root, target string) bool {
	root, _ = filepath.Abs(root)
	target, _ = filepath.Abs(target)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || filepath.IsAbs(rel) {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
