//go:build windows

package native

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// DisconnectCandidate induces transport loss only in the sole app-server child
// of the original public Runner daemon. It is never a native stop receipt.
func DisconnectCandidate(target ProcessFaultTarget, ownedRoot string) error {
	relative, err := filepath.Rel(ownedRoot, target.Workspace)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) || target.ParentPID <= 0 || target.ParentCreated == "" || !filepath.IsAbs(target.Executable) || target.Session == "" {
		return errors.New("fault target is not one owned local Run")
	}
	parent, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(target.ParentPID))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(parent)
	created := func(handle windows.Handle) (string, error) {
		var c, e, k, u windows.Filetime
		if err := windows.GetProcessTimes(handle, &c, &e, &k, &u); err != nil {
			return "", err
		}
		return strconv.FormatUint(uint64(c.HighDateTime)<<32|uint64(c.LowDateTime), 10), nil
	}
	parentCreated, err := created(parent)
	if err != nil || parentCreated != target.ParentCreated {
		return errors.New("original Runner parent identity changed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	script := `[Console]::InputEncoding=[Text.UTF8Encoding]::new($false);[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false);$r=[Console]::In.ReadToEnd()|ConvertFrom-Json;$items=@(Get-CimInstance Win32_Process -Filter ('ParentProcessId='+[int]$r.pid)|Where-Object {$_.ExecutablePath -eq $r.executable -and $_.CommandLine -match '(?i)app-server'}|ForEach-Object {@{pid=[int]$_.ProcessId;executable=$_.ExecutablePath;created=$_.CreationDate.ToFileTimeUtc().ToString()}});[Console]::Out.Write((ConvertTo-Json -InputObject $items -Compress))`
	body, _ := json.Marshal(map[string]any{"pid": target.ParentPID, "executable": target.Executable})
	command := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	command.Stdin = bytes.NewReader(body)
	raw, err := command.Output()
	if err != nil {
		return fmt.Errorf("owned child inventory unavailable: %w", err)
	}
	var children []struct {
		PID        int    `json:"pid"`
		Executable string `json:"executable"`
		Created    string `json:"created"`
	}
	if len(raw) > 64<<10 || json.Unmarshal(raw, &children) != nil || len(children) != 1 {
		return errors.New("original Runner does not have one unambiguous selected app-server child")
	}
	child := children[0]
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(child.PID))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	observed, err := created(h)
	observedTicks, observedErr := strconv.ParseUint(observed, 10, 64)
	childTicks, childErr := strconv.ParseUint(child.Created, 10, 64)
	// CIM DMTF timestamps have microsecond precision; retained process handles
	// provide 100 ns FILETIME. Compare the same precision, then keep the handle.
	if err != nil || observedErr != nil || childErr != nil || observedTicks/10 != childTicks/10 {
		return errors.New("native fault PID creation identity changed")
	}
	image := make([]uint16, 32768)
	size := uint32(len(image))
	if err := windows.QueryFullProcessImageName(h, 0, &image[0], &size); err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(windows.UTF16ToString(image[:size])), filepath.Clean(target.Executable)) {
		return errors.New("native fault image changed")
	}
	if status, err := windows.WaitForSingleObject(parent, 0); err != nil || status != uint32(windows.WAIT_TIMEOUT) {
		return errors.New("original Runner parent exited during child verification")
	}
	if err := windows.TerminateProcess(h, 1); err != nil {
		return err
	}
	status, err := windows.WaitForSingleObject(h, 5000)
	if err != nil {
		return err
	}
	if status != windows.WAIT_OBJECT_0 {
		return errors.New("original fault target termination unverified")
	}
	return nil
}
