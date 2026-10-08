//go:build unix

package pathguard

import (
	"os"
	"syscall"
)

func fileLinkCount(path string) (uint64, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return uint64(info.Sys().(*syscall.Stat_t).Nlink), nil
}
