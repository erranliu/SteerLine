//go:build !windows && !linux

package execution

import (
	"fmt"
	"os"
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
	return f, fmt.Sprintf("%s:%d:%d", path, info.ModTime().UnixNano(), info.Size()), nil
}
