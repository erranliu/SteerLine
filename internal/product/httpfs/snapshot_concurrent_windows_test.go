//go:build windows

package httpfs

import (
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"testing"
)

// Owned snapshot readers must not exclude publication. Use the single rename
// primitive so replacement retries cannot conceal a conflicting reader.
func TestPrivateSnapshotConcurrentPublication(t *testing.T) {
	root := t.TempDir()
	if err := EnsurePrivateDirectory(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "receipt.json")
	write := func(n int) error {
		f, err := CreatePrivateTemp(root, ".discovery-")
		if err != nil {
			return err
		}
		if _, err = fmt.Fprintf(f, `{"revision":%d}`, n); err != nil {
			f.Close()
			return err
		}
		if err = f.Close(); err != nil {
			return err
		}
		return renamePrivateSnapshot(f.Name(), path)
	}
	if err := write(0); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	failures := make(chan error, 8)
	var readers sync.WaitGroup
	for i := 0; i < 8; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				f, err := OpenPrivateSnapshot(path)
				if err != nil {
					failures <- err
					return
				}
				_, err = io.Copy(io.Discard, f)
				f.Close()
				if err != nil {
					failures <- err
					return
				}
			}
		}()
	}
	for i := 1; i <= 1000; i++ {
		if err := write(i); err != nil {
			close(done)
			readers.Wait()
			t.Fatalf("publication %d: %v", i, err)
		}
	}
	close(done)
	readers.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
}
