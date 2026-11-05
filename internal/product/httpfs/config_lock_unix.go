//go:build !windows

package httpfs

import (
	"os"
	"path/filepath"
	"syscall"
)

func WithConfigurationLock(home string, action func() error) error {
	p := filepath.Join(home, ".configuration.lock")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err = verifyPrivateSnapshot(f); err != nil {
		return err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return action()
}
