package testprotocol

import (
	"errors"
	"golang.org/x/sys/windows"
)

// A cancelled fixture command may be killed before its deferred unlock runs.
// Exclusive sharing serializes access; delete-on-close releases the lock even
// when Windows closes the handle on process termination.
func tryNativeOrcaStateLock(path string) (func(), error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_DELETE_ON_CLOSE, 0)
	if errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return func() { _ = windows.CloseHandle(handle) }, nil
}
