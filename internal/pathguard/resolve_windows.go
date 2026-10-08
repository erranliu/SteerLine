//go:build windows

package pathguard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// ResolveProspective resolves existing paths through Windows reparse points
// (including junctions) and appends a missing tail for write destinations.
func ResolveProspective(input string) (string, error) {
	path, err := filepath.Abs(input)
	if err != nil {
		return "", err
	}
	path = filepath.Clean(path)
	volume := filepath.VolumeName(path)
	if volume == "" {
		return "", fmt.Errorf("Windows path has no volume: %s", path)
	}
	root := volume
	if !strings.HasSuffix(root, string(filepath.Separator)) {
		root += string(filepath.Separator)
	}
	remainder := strings.TrimLeft(path[len(volume):], `\/`)
	if remainder == "" {
		return filepath.Clean(root), nil
	}
	parts := strings.FieldsFunc(remainder, func(r rune) bool { return r == '\\' || r == '/' })
	current := filepath.Clean(root)
	for index, part := range parts {
		candidate := filepath.Join(current, part)
		attributes, attributeErr := fileAttributes(candidate)
		if attributeErr != nil {
			if !errors.Is(attributeErr, windows.ERROR_FILE_NOT_FOUND) && !errors.Is(attributeErr, windows.ERROR_PATH_NOT_FOUND) {
				return "", attributeErr
			}
			if isUnresolvedReparsePoint(candidate) {
				return "", fmt.Errorf("cannot resolve Windows reparse point: %s", candidate)
			}
			for _, tail := range parts[index:] {
				current = filepath.Join(current, tail)
			}
			return filepath.Clean(current), nil
		}
		info, statErr := os.Lstat(candidate)
		isReparse := attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || statErr == nil && info.Mode()&os.ModeSymlink != 0
		if isReparse {
			current, err = finalPath(candidate)
			if err != nil {
				return "", fmt.Errorf("resolve Windows reparse point %s: %w", candidate, err)
			}
		} else {
			current = candidate
		}
		if index < len(parts)-1 && attributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
			return "", fmt.Errorf("parent of prospective path is not a directory: %s", candidate)
		}
	}
	return filepath.Clean(current), nil
}

func fileAttributes(path string) (uint32, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	return windows.GetFileAttributes(name)
}

func finalPath(path string) (string, error) {
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
			return "", errors.New("GetFinalPathNameByHandle returned an empty path")
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

func isUnresolvedReparsePoint(path string) bool {
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
