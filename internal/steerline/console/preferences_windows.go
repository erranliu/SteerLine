//go:build windows

package console

import (
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

type viewLock struct {
	f *os.File
	o windows.Overlapped
}

func lockView(dir string) (*viewLock, error) {
	f, err := os.OpenFile(filepath.Join(dir, "steerline.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	l := &viewLock{f: f}
	err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &l.o)
	if err != nil {
		f.Close()
		return nil, err
	}
	return l, nil
}
func (l *viewLock) Close() {
	windows.UnlockFileEx(windows.Handle(l.f.Fd()), 0, 1, 0, &l.o)
	l.f.Close()
}
func replaceViewFile(from, to string) error {
	a, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	b, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 100; attempt++ {
		err = windows.MoveFileEx(a, b, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		if err != windows.ERROR_SHARING_VIOLATION && err != windows.ERROR_ACCESS_DENIED {
			return err
		}
		time.Sleep(5 * time.Millisecond)
	}
	return err
}
