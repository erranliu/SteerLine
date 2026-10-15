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

// Only OS process generation and existing advisory locks are observed here.
// No checkpoint, SQL, inbox or command receipt is read.
func observeExit(identity wire.Identity) (bool, bool, error) {
	exited, err := ProcessExited(identity.Process)
	if err != nil || !exited {
		return exited, false, err
	}
	if len(identity.LockPaths) != 2 {
		return true, false, errors.New("original service locks unavailable")
	}
	for _, path := range identity.LockPaths {
		if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Base(path), "steerline.lock") {
			return true, false, errors.New("invalid original lock path")
		}
		info, err := os.Lstat(path)
		if err != nil {
			return true, false, err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return true, false, errors.New("linked/nonregular lock path")
		}
		file, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			return true, false, err
		}
		var overlap windows.Overlapped
		err = windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlap)
		if err != nil {
			file.Close()
			if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
				return true, false, nil
			}
			return true, false, err
		}
		err = windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlap)
		file.Close()
		if err != nil {
			return true, false, err
		}
	}
	return true, true, nil
}

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
