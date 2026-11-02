//go:build windows

package httpfs

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Coordinate only namespace lookup/publication. A returned file handle never
// holds this lock, so immutable readers do not exclude subsequent publication.
// The named kernel object also coordinates independent caller processes; it
// leaves no sidecar file and does not retry a failed filesystem operation.
// One namespace lock per user also covers extended, short and junction path
// aliases without trying to infer filesystem identity from a path string.
func lockSnapshotNamespace() (func() error, error) {
	user, err := CurrentUserSID()
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(user.String()))
	name, err := windows.UTF16PtrFromString(fmt.Sprintf("Global\\SteerLine.Snapshot.%x", digest))
	if err != nil {
		return nil, err
	}
	sd, err := windows.SecurityDescriptorFromString(fmt.Sprintf("O:%sD:P(A;;0x%x;;;%s)", user.String(), windows.MUTEX_ALL_ACCESS, user.String()))
	if err != nil {
		return nil, err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	mutex, err := windows.CreateMutex(&sa, false, name)
	runtime.KeepAlive(sd)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil, err
	}
	security, err := windows.GetSecurityInfo(mutex, windows.SE_KERNEL_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err == nil {
		err = verifyPrivateDescriptorAccess(security, user, windows.ACCESS_MASK(windows.MUTEX_ALL_ACCESS))
	}
	if err != nil {
		windows.CloseHandle(mutex)
		return nil, fmt.Errorf("snapshot namespace lock identity: %w", err)
	}
	// Windows mutex ownership belongs to a native thread, not a Go goroutine.
	runtime.LockOSThread()
	status, err := windows.WaitForSingleObject(mutex, 5000)
	if err != nil || status != windows.WAIT_OBJECT_0 && status != windows.WAIT_ABANDONED {
		windows.CloseHandle(mutex)
		runtime.UnlockOSThread()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("snapshot namespace lock wait returned %#x", status)
	}
	return func() error {
		err := windows.ReleaseMutex(mutex)
		err = errors.Join(err, windows.CloseHandle(mutex))
		runtime.UnlockOSThread()
		return err
	}, nil
}
