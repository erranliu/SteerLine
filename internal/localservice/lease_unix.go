//go:build !windows

package localservice

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

type lease struct{ f *os.File }

func lockLease(dir string) (*lease, error) {
	f, err := os.OpenFile(filepath.Join(dir, "steerline.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, err
	}
	return &lease{f: f}, nil
}
func (l *lease) Close() { l.f.Close() }

func leaseBusy(err error) bool {
	return errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN)
}
