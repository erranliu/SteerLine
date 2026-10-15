//go:build windows

package localservice

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

type lease struct {
	f *os.File
	o windows.Overlapped
}

func lockLease(dir string) (*lease, error) {
	f, err := os.OpenFile(filepath.Join(dir, "steerline.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	l := &lease{f: f}
	err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &l.o)
	if err != nil {
		f.Close()
		return nil, err
	}
	return l, nil
}
func (l *lease) Close() {
	windows.UnlockFileEx(windows.Handle(l.f.Fd()), 0, 1, 0, &l.o)
	l.f.Close()
}

func leaseBusy(err error) bool { return errors.Is(err, windows.ERROR_LOCK_VIOLATION) }
