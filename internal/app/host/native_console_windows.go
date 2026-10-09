//go:build windows

package host

import (
	"context"
	"strings"
	"syscall"
	"unsafe"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"golang.org/x/sys/windows"
)

// OpenNativeConsole creates an interactive console with its own standard
// handles. It launches only the viewer; the worker remains service-owned.
func OpenNativeConsole(ctx context.Context, command executionbackend.LaunchCommand) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	executable, err := windows.UTF16PtrFromString(command.Executable)
	if err != nil {
		return err
	}
	directory, err := windows.UTF16PtrFromString(command.WorkingDirectory)
	if err != nil {
		return err
	}
	args := []string{syscall.EscapeArg(command.Executable)}
	for _, arg := range command.Args {
		args = append(args, syscall.EscapeArg(arg))
	}
	line, err := windows.UTF16PtrFromString(strings.Join(args, " "))
	if err != nil {
		return err
	}
	startup := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var process windows.ProcessInformation
	if err := windows.CreateProcess(executable, line, nil, nil, false, windows.CREATE_NEW_CONSOLE, nil, directory, &startup, &process); err != nil {
		return err
	}
	windows.CloseHandle(process.Thread)
	windows.CloseHandle(process.Process)
	return nil
}
