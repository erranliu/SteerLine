//go:build windows

package localservice

import (
	"fmt"
	"golang.org/x/sys/windows"
	"unsafe"
)

// VerifyDetachedLifetime runs in the resident child before any API or project
// resources are opened. Membership in a permanent/non-killing system Job is
// harmless; a caller-owned kill-on-close Job cannot own a detached product.
func VerifyDetachedLifetime() error {
	var inJob uint32
	ok, _, err := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob").Call(uintptr(windows.CurrentProcess()), 0, uintptr(unsafe.Pointer(&inJob)))
	if ok == 0 {
		return fmt.Errorf("verify resident Job membership: %w", err)
	}
	if inJob == 0 {
		return nil
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if err := windows.QueryInformationJobObject(0, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)), nil); err != nil {
		return err
	}
	if limits.BasicLimitInformation.LimitFlags&windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE != 0 {
		return fmt.Errorf("resident remains in kill-on-close Job (flags=0x%x); refusing resource startup", limits.BasicLimitInformation.LimitFlags)
	}
	return nil
}
