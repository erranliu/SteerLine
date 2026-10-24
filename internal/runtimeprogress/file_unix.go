//go:build !windows

package runtimeprogress

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func lockFile(f *os.File) error {
	return unix.FcntlFlock(f.Fd(), unix.F_SETLK, &unix.Flock_t{Type: unix.F_WRLCK, Whence: 0, Start: 0, Len: 1})
}
func unlockFile(f *os.File) {
	_ = unix.FcntlFlock(f.Fd(), unix.F_SETLK, &unix.Flock_t{Type: unix.F_UNLCK, Whence: 0, Start: 0, Len: 1})
}
func lockBusy(err error) bool           { return errors.Is(err, unix.EACCES) || errors.Is(err, unix.EAGAIN) }
func replaceFile(from, to string) error { return os.Rename(from, to) }
