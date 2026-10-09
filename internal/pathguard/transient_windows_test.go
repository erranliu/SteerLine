//go:build windows

package pathguard

import (
	"errors"
	"golang.org/x/sys/windows"
	"testing"
)

func TestTransientFileInspectionStillFailsClosed(t *testing.T) {
	for _, transient := range []error{windows.ERROR_ACCESS_DENIED, windows.ERROR_SHARING_VIOLATION} {
		calls := 0
		value, err := retryFileInspection(func() (int, error) {
			calls++
			if calls < 3 {
				return 0, transient
			}
			return 2, nil
		})
		if err != nil || value != 2 || calls != 3 {
			t.Fatal("transient inspection did not retain its actual result", value, calls, err)
		}
		calls = 0
		_, err = retryFileInspection(func() (int, error) { calls++; return 0, transient })
		if !errors.Is(err, transient) || calls != 20 {
			t.Fatal("persistent permission error was admitted", calls, err)
		}
	}
	calls := 0
	_, err := retryFileInspection(func() (int, error) { calls++; return 0, windows.ERROR_FILE_NOT_FOUND })
	if !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || calls != 1 {
		t.Fatal("missing file handling changed", calls, err)
	}
}
