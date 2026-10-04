//go:build windows

package controlled

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

func replaceStateFile(from, to string) error {
	var err error
	// Readers such as PowerShell may omit FILE_SHARE_DELETE. Preserve the last
	// complete snapshot while a short-lived reader prevents its replacement.
	for attempt := 0; attempt < 100; attempt++ {
		err = os.Rename(from, to)
		if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return err
		}
		time.Sleep(5 * time.Millisecond)
	}
	return err
}
