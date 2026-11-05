//go:build windows

package httpfs

import "errors"

// WithConfigurationLock holds the current-user namespace mutex across a
// configuration read/compare/publication transaction, including other processes.
func WithConfigurationLock(home string, action func() error) (err error) {
	unlock, err := lockSnapshotNamespace()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, unlock()) }()
	return action()
}
