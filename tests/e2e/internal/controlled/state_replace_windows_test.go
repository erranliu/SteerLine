//go:build windows

package controlled

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWritePreservesSnapshotUntilWindowsReaderReleases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := Write(path, State{Dispatches: 1}); err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if handle != windows.InvalidHandle {
			windows.CloseHandle(handle)
		}
	}()
	done := make(chan error, 1)
	go func() { done <- Write(path, State{Dispatches: 2}) }()
	select {
	case err := <-done:
		t.Fatalf("write completed while replacement was locked: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	before, err := Read(path)
	if err != nil || before.Dispatches != 1 {
		t.Fatalf("previous snapshot was lost: %+v, %v", before, err)
	}
	if err := windows.CloseHandle(handle); err != nil {
		t.Fatal(err)
	}
	handle = windows.InvalidHandle
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("write did not finish after reader released")
	}
	after, err := Read(path)
	if err != nil || after.Dispatches != 2 {
		t.Fatalf("replacement snapshot differs: %+v, %v", after, err)
	}
	files, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(files) != 1 {
		t.Fatalf("temporary files remain: %v, %v", files, err)
	}
}
