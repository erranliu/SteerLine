//go:build !windows

package localservice

import (
	"os"
	"path/filepath"
	"strconv"
)

func ExecutableRunning(path string) (bool, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if _, e := strconv.Atoi(entry.Name()); e != nil {
			continue
		}
		p, e := os.Readlink(filepath.Join("/proc", entry.Name(), "exe"))
		if e == nil && p == path {
			return true, nil
		}
	}
	return false, nil
}
