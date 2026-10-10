//go:build windows

package pathguard

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

func fileLinkCount(path string) (uint64, error) {
	attempts := 1
	if sqliteSidecar(path) {
		// The Windows SQLite VFS closes and removes -shm when its last
		// connection unmaps it. A path-based link-count open can overlap that
		// delete-pending window and receive ERROR_ACCESS_DENIED. Recheck only
		// sidecars for a short bounded interval; persistent ACL denial still
		// returns the original error below.
		attempts = 5
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		links, err := fileLinkCountOnce(path)
		if err == nil {
			return links, nil
		}
		lastErr = err
		if !errors.Is(err, windows.ERROR_ACCESS_DENIED) || attempt+1 == attempts {
			return 0, err
		}
		time.Sleep(time.Duration(attempt+1) * time.Millisecond)
	}
	return 0, lastErr
}

func fileLinkCountOnce(path string) (uint64, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	handle, err := retryFileInspection(func() (windows.Handle, error) {
		return windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
			nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	})
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("open SQLite state file for link-count inspection: %w", err)
	}
	defer windows.CloseHandle(handle)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return 0, fmt.Errorf("read SQLite state file link count: %w", err)
	}
	return uint64(info.NumberOfLinks), nil
}

func sqliteSidecar(path string) bool {
	name := filepath.Base(path)
	return strings.HasSuffix(name, "-wal") || strings.HasSuffix(name, "-shm") || strings.HasSuffix(name, "-journal")
}
