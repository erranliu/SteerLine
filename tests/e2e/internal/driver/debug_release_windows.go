//go:build windows

package driver

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"time"
)

func debugPlainFile(path string) error {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	// Metadata-only access avoids interfering with SQLite's live shared-memory handle.
	var handle windows.Handle
	for attempt := 0; attempt < 40; attempt++ {
		handle, err = windows.CreateFile(name, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
		if err == nil || (!errors.Is(err, windows.ERROR_ACCESS_DENIED) && !errors.Is(err, windows.ERROR_SHARING_VIOLATION)) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		return &os.PathError{Op: "inspect debug file", Path: path, Err: err}
	}
	defer windows.CloseHandle(handle)
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(handle, &info); err != nil {
		return err
	}
	if info.NumberOfLinks > 1 {
		return fmt.Errorf("hard-linked debug file: %s", path)
	}
	return nil
}

func debugVisibleConsole() bool {
	hwnd, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow").Call()
	if hwnd == 0 {
		return false
	}
	visible, _, _ := windows.NewLazySystemDLL("user32.dll").NewProc("IsWindowVisible").Call(hwnd)
	return visible != 0
}
