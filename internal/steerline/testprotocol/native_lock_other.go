//go:build !windows

package testprotocol

import "os"

func tryNativeOrcaStateLock(path string) (func(), error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if os.IsExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = file.Close()
	return func() { _ = os.Remove(path) }, nil
}
