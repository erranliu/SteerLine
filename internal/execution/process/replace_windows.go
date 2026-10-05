//go:build windows

package process

import (
	"time"

	"golang.org/x/sys/windows"
)

func replaceFile(from, to string) error {
	fromPtr, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	toPtr, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 100; attempt++ {
		err = windows.MoveFileEx(fromPtr, toPtr, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		if err != windows.ERROR_SHARING_VIOLATION && err != windows.ERROR_ACCESS_DENIED {
			return err
		}
		time.Sleep(5 * time.Millisecond)
	}
	return err
}
