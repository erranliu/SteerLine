//go:build !windows

package console

import (
	"os"
	"path/filepath"
)

func resolveConsolePath(input string) (string, error) {
	path, err := filepath.Abs(input)
	if err != nil {
		return "", err
	}
	var tail []string
	for {
		resolved, resolveErr := filepath.EvalSymlinks(path)
		if resolveErr == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(resolveErr) {
			return "", resolveErr
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", resolveErr
		}
		tail = append(tail, filepath.Base(path))
		path = parent
	}
}
