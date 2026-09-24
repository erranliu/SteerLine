package steerline

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestAtomicWriteENOSPCPreservesPreviousSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runtime.json")
	if err := AtomicWrite(path, []byte("previous snapshot")); err != nil {
		t.Fatal(err)
	}
	err := atomicWriteWith(path, []byte("replacement snapshot"), func(file *os.File, data []byte) error {
		if _, writeErr := file.Write(data[:5]); writeErr != nil {
			return writeErr
		}
		return syscall.ENOSPC
	})
	if !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("partial disk-full write returned %v", err)
	}
	previous, err := os.ReadFile(path)
	if err != nil || string(previous) != "previous snapshot" {
		t.Fatalf("disk-full write replaced the prior snapshot: %q %v", previous, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".steerline-") && strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("disk-full write left a temporary snapshot: %s", entry.Name())
		}
	}
}
