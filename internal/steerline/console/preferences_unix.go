//go:build !windows

package console

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

type viewLock struct{ f *os.File }

func lockView(dir string) (*viewLock, error) {
	f, err := os.OpenFile(filepath.Join(dir, "steerline.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, err
	}
	return &viewLock{f: f}, nil
}
func (l *viewLock) Close()                  { l.f.Close() }
func replaceViewFile(from, to string) error { return os.Rename(from, to) }
