//go:build windows

package httpclient

import (
	"errors"
	"fmt"
	wire "github.com/erranliu/SteerLine/internal/product/httpcontract"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
)

// ProcessExited observes the exact OS generation without reading product state.
func ProcessExited(old wire.ProcessIdentity) (bool, error) {
	if old.Kind != "os" || old.PID <= 0 || old.Created == "" || old.Executable == "" {
		return false, errors.New("original process identity is unverifiable")
	}
	exited := false
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(old.PID))
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		exited = true
	} else if err != nil {
		return false, err
	} else {
		defer windows.CloseHandle(handle)
		status, err := windows.WaitForSingleObject(handle, 0)
		if err != nil {
			return false, err
		}
		if status == windows.WAIT_OBJECT_0 {
			exited = true
		} else {
			var created, exit, kernel, user windows.Filetime
			if err = windows.GetProcessTimes(handle, &created, &exit, &kernel, &user); err != nil {
				return false, err
			}
			buf := make([]uint16, 32768)
			n := uint32(len(buf))
			if err = windows.QueryFullProcessImageName(handle, 0, &buf[0], &n); err != nil {
				return false, err
			}
			exited = !strings.EqualFold(filepath.Clean(windows.UTF16ToString(buf[:n])), filepath.Clean(old.Executable)) || fmt.Sprintf("%08x%08x", created.HighDateTime, created.LowDateTime) != old.Created
		}
	}
	return exited, nil
}
func CurrentProcessIdentity() (wire.ProcessIdentity, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(os.Getpid()))
	if err != nil {
		return wire.ProcessIdentity{}, err
	}
	defer windows.CloseHandle(handle)
	var created, exit, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(handle, &created, &exit, &kernel, &user); err != nil {
		return wire.ProcessIdentity{}, err
	}
	buf := make([]uint16, 32768)
	n := uint32(len(buf))
	if err = windows.QueryFullProcessImageName(handle, 0, &buf[0], &n); err != nil {
		return wire.ProcessIdentity{}, err
	}
	return wire.ProcessIdentity{Kind: "os", PID: os.Getpid(), Created: fmt.Sprintf("%08x%08x", created.HighDateTime, created.LowDateTime), Executable: strings.ToLower(filepath.Clean(windows.UTF16ToString(buf[:n])))}, nil
}
