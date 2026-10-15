//go:build windows

package driver

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// NativeSandboxProbe runs inside the native tool's token. It does not assume
// that MXC preserves the app-server's host TEMP path or container directory name.
func NativeSandboxProbe(args []string) error {
	if len(args) != 1 {
		return errors.New("sandbox probe requires the generated negative sentinel")
	}
	sentinel := filepath.Clean(args[0])
	if !filepath.IsAbs(sentinel) || !strings.HasPrefix(strings.ToLower(sentinel), `c:\steerlinedebug\`) || !strings.EqualFold(filepath.Base(sentinel), "sentinel.txt") || !strings.EqualFold(filepath.Base(filepath.Dir(sentinel)), "sandbox-negative") {
		return errors.New("negative sentinel must belong to a generated SteerlineDebug package")
	}
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return err
	}
	defer token.Close()
	var appContainer, size uint32
	// TOKEN_INFORMATION_CLASS.TokenIsAppContainer is 29 in the Windows SDK.
	if err := windows.GetTokenInformation(token, 29, (*byte)(unsafe.Pointer(&appContainer)), 4, &size); err != nil {
		return err
	}
	if appContainer == 0 || size != 4 {
		return errors.New("native tool is not executing with an AppContainer token")
	}
	containerRoot, err := currentAppContainerFolder(token)
	if err != nil {
		return err
	}
	temp, tmp := os.Getenv("TEMP"), os.Getenv("TMP")
	if !filepath.IsAbs(temp) || !filepath.IsAbs(tmp) || !strings.EqualFold(filepath.Clean(temp), filepath.Clean(tmp)) {
		return errors.New("native sandbox TEMP/TMP are missing or inconsistent")
	}
	rel, err := filepath.Rel(containerRoot, temp)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return errors.New("native tool TEMP is outside its OS AppContainer folder")
	}
	probe, err := os.CreateTemp(temp, "steerrunner-sandbox-probe-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(probe.Name())
	if _, err := probe.WriteString("native sandbox temp round trip"); err != nil {
		probe.Close()
		return err
	}
	if err := probe.Close(); err != nil {
		return err
	}
	body, err := os.ReadFile(probe.Name())
	if err != nil || string(body) != "native sandbox temp round trip" {
		return errors.New("native sandbox temporary file verification failed")
	}
	if _, err := os.Stat(sentinel); err != nil {
		return fmt.Errorf("negative sentinel is unavailable: %w", err)
	}
	file, err := os.OpenFile(sentinel, os.O_WRONLY, 0)
	if err == nil {
		file.Close()
		return errors.New("native sandbox allowed sibling write access")
	}
	if !errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("sibling write denial was not verified: %w", err)
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"passed": true, "app_container": true, "temp_in_kernel": true, "sibling_write_denied": true})
}

func currentAppContainerFolder(token windows.Token) (string, error) {
	var size uint32
	if err := windows.GetTokenInformation(token, 31, nil, 0, &size); err != windows.ERROR_INSUFFICIENT_BUFFER || size < uint32(unsafe.Sizeof(uintptr(0))) {
		return "", fmt.Errorf("AppContainer SID size unavailable: %v", err)
	}
	buffer := make([]byte, size)
	if err := windows.GetTokenInformation(token, 31, &buffer[0], size, &size); err != nil {
		return "", err
	}
	sid := *(**windows.SID)(unsafe.Pointer(&buffer[0]))
	if sid == nil {
		return "", errors.New("AppContainer SID is missing")
	}
	text, err := windows.UTF16PtrFromString(sid.String())
	if err != nil {
		return "", err
	}
	getFolder := windows.NewLazySystemDLL("userenv.dll").NewProc("GetAppContainerFolderPath")
	var folder *uint16
	result, _, _ := getFolder.Call(uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(&folder)))
	runtime.KeepAlive(buffer)
	if uint32(result) != 0 || folder == nil {
		return "", fmt.Errorf("OS AppContainer folder lookup failed: 0x%x", uint32(result))
	}
	defer windows.NewLazySystemDLL("ole32.dll").NewProc("CoTaskMemFree").Call(uintptr(unsafe.Pointer(folder)))
	return filepath.Clean(windows.UTF16PtrToString(folder)), nil
}
