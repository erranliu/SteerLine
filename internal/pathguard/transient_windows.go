//go:build windows

package pathguard

import (
	"errors"
	"golang.org/x/sys/windows"
	"time"
)

// Delete-pending SQLite sidecars may briefly return access/sharing errors even
// with FILE_SHARE_DELETE. Retry the same inspection; never admit an unread file.
func retryFileInspection[T any](read func() (T, error)) (T, error) {
	var value T
	var err error
	for attempt := 0; attempt < 20; attempt++ {
		value, err = read()
		if err == nil || (!errors.Is(err, windows.ERROR_ACCESS_DENIED) && !errors.Is(err, windows.ERROR_SHARING_VIOLATION)) {
			return value, err
		}
		if attempt < 19 {
			time.Sleep(5 * time.Millisecond)
		}
	}
	return value, err
}
