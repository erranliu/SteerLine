//go:build windows

package console

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func resolveConsolePath(input string) (string, error) {
	path, err := filepath.Abs(input)
	if err != nil {
		return "", err
	}
	path = filepath.Clean(path)
	var tail []string
	for {
		resolved, resolveErr := finalConsolePath(path)
		if resolveErr == nil {
			if len(tail) > 0 {
				info, statErr := os.Stat(resolved)
				if statErr != nil {
					return "", statErr
				}
				if !info.IsDir() {
					return "", fmt.Errorf("parent of prospective UI path is not a directory: %s", resolved)
				}
			}
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(resolveErr, windows.ERROR_FILE_NOT_FOUND) && !errors.Is(resolveErr, windows.ERROR_PATH_NOT_FOUND) {
			return "", resolveErr
		}
		if unresolvedConsoleReparsePoint(path) {
			return "", fmt.Errorf("cannot resolve Windows UI reparse point: %s", path)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", resolveErr
		}
		tail = append(tail, filepath.Base(path))
		path = parent
	}
}

func finalConsolePath(path string) (string, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	handle, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, 256)
	for {
		n, callErr := windows.GetFinalPathNameByHandle(handle, &buffer[0], uint32(len(buffer)), 0)
		if callErr != nil {
			return "", callErr
		}
		if n == 0 {
			return "", errors.New("GetFinalPathNameByHandle returned an empty UI path")
		}
		if int(n) >= len(buffer) {
			buffer = make([]uint16, int(n)+1)
			continue
		}
		resolved := windows.UTF16ToString(buffer[:n])
		if strings.HasPrefix(strings.ToUpper(resolved), `\\?\UNC\`) {
			resolved = `\\` + resolved[8:]
		} else if strings.HasPrefix(resolved, `\\?\`) {
			resolved = resolved[4:]
		}
		return filepath.Clean(resolved), nil
	}
}

func unresolvedConsoleReparsePoint(path string) bool {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	attributes, err := windows.GetFileAttributes(name)
	return err == nil && attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
