//go:build linux

package steerline

import (
	"fmt"
	"os"
	"syscall"
)

func openJournal(path string) (*os.File, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, "", err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		f.Close()
		return nil, "", fmt.Errorf("journal file identity unavailable for %s", path)
	}
	return f, fmt.Sprintf("%x:%x", uint64(st.Dev), uint64(st.Ino)), nil
}
