//go:build windows

package runtimeprogress

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"time"
)

func lockFile(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
}
func unlockFile(f *os.File) {
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &windows.Overlapped{})
}
func lockBusy(err error) bool { return errors.Is(err, windows.ERROR_LOCK_VIOLATION) }
func replaceFile(from, to string) error {
	var err error
	for i := 0; i < 30; i++ {
		err = os.Rename(from, to)
		if err == nil || !(errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_ACCESS_DENIED)) {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	return err
}
