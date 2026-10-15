//go:build windows

package localservice

import (
	"errors"
	"golang.org/x/sys/windows"
	"path/filepath"
	"strings"
	"unsafe"
)

func ExecutableRunning(path string) (bool, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if !strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), filepath.Base(path)) {
			continue
		}
		handle, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if e != nil {
			if errors.Is(e, windows.ERROR_INVALID_PARAMETER) {
				continue
			}
			return false, e
		}
		buf := make([]uint16, 32768)
		n := uint32(len(buf))
		e = windows.QueryFullProcessImageName(handle, 0, &buf[0], &n)
		windows.CloseHandle(handle)
		if e != nil {
			return false, e
		}
		if strings.EqualFold(filepath.Clean(windows.UTF16ToString(buf[:n])), filepath.Clean(path)) {
			return true, nil
		}
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return false, err
	}
	return false, nil
}
