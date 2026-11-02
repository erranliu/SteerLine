//go:build windows

package httpfs

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestPrivateSnapshotValidatesTheOpenedFileAcrossPathReplacement(t *testing.T) {
	root := t.TempDir()
	if err := EnsurePrivateDirectory(root); err != nil {
		t.Fatal(err)
	}
	write := func(body string) string {
		f, err := CreatePrivateTemp(root, ".snapshot-")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.WriteString(body); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
		return f.Name()
	}
	path := filepath.Join(root, "receipt.json")
	if err := renamePrivateSnapshot(write("original"), path); err != nil {
		t.Fatal(err)
	}
	// Hold the exact point between opening a file and validating its security.
	// A replacement must neither invalidate that file nor validate a new file
	// using the old file's ACL.
	snapshot, err := openDiscoveryReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	if err = renamePrivateSnapshot(write("replacement"), path); err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	if err = verifyPrivateSnapshot(snapshot); err != nil {
		t.Fatal("replacement path changed original snapshot security", err)
	}
	if file, err := OpenPrivateSnapshot(path); err == nil {
		file.Close()
		t.Fatal("replacement inherited the previous file's trust")
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = verifyPrivateSnapshot(snapshot); err != nil {
		t.Fatal("unlinked snapshot could not be verified", err)
	}
	body, err := io.ReadAll(snapshot)
	if err != nil || string(body) != "original" {
		t.Fatal("opened snapshot changed", string(body), err)
	}
}

func TestPrivateSnapshotsCoordinateAcrossProcesses(t *testing.T) {
	write := func(path string, revision int) {
		f, err := CreatePrivateTemp(filepath.Dir(path), ".snapshot-")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = fmt.Fprintf(f, `{"revision":%d}`, revision); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
		if err = renamePrivateSnapshot(f.Name(), path); err != nil {
			t.Fatal(err)
		}
	}
	if path := os.Getenv("STEERLINE_HTTPFS_SNAPSHOT_CHILD"); path != "" {
		fmt.Fprintln(os.Stdout, "snapshot-writer-ready")
		if _, err := io.ReadFull(os.Stdin, make([]byte, 1)); err != nil {
			t.Fatal(err)
		}
		for i := 1; i <= 1000; i++ {
			write(path, i)
		}
		return
	}
	root := t.TempDir()
	if err := EnsurePrivateDirectory(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "receipt.json")
	write(path, 0)
	// Readers deliberately use a different spelling of the writer's path.
	readPath := `\\?\` + path
	if strings.HasPrefix(path, `\\`) {
		readPath = `\\?\UNC\` + strings.TrimPrefix(path, `\\`)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestPrivateSnapshotsCoordinateAcrossProcesses$", "-test.count=1")
	cmd.Env = append(os.Environ(), "STEERLINE_HTTPFS_SNAPSHOT_CHILD="+path)
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	})
	reader := bufio.NewReader(output)
	if ready, err := reader.ReadString('\n'); err != nil || ready != "snapshot-writer-ready\n" {
		t.Fatal("child writer not ready", ready, err)
	}
	done, entered := make(chan struct{}), make(chan error, 8)
	failures := make(chan error, 8)
	readSnapshot := func() error {
		file, err := OpenPrivateSnapshot(readPath)
		if err != nil {
			return err
		}
		_, err = io.Copy(io.Discard, file)
		closeErr := file.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	var readers sync.WaitGroup
	for i := 0; i < 8; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			err := readSnapshot()
			entered <- err
			if err != nil {
				return
			}
			for {
				select {
				case <-done:
					return
				default:
				}
				if err := readSnapshot(); err != nil {
					failures <- err
					return
				}
			}
		}()
	}
	for i := 0; i < 8; i++ {
		if err := <-entered; err != nil {
			t.Error("reader did not complete its first read", err)
		}
	}
	if t.Failed() {
		close(done)
		readers.Wait()
		t.FailNow()
	}
	_, startErr := input.Write([]byte{1})
	input.Close()
	// Drain the finite test output while Wait owns the child lifecycle.
	io.Copy(io.Discard, reader)
	waitErr := cmd.Wait()
	close(done)
	readers.Wait()
	close(failures)
	if startErr != nil || waitErr != nil {
		t.Fatal("child publication failed", startErr, waitErr)
	}
	for err := range failures {
		t.Error(err)
	}
}
