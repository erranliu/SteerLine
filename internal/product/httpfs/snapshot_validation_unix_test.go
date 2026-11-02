//go:build !windows

package httpfs

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestPrivateSnapshotValidatesTheOpenedFileAcrossPathReplacement(t *testing.T) {
	root := t.TempDir()
	path, replacement := filepath.Join(root, "snapshot"), filepath.Join(root, "replacement")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked")
	if err := os.Link(path, link); err != nil {
		t.Fatal(err)
	}
	if file, err := OpenPrivateSnapshot(path); err == nil {
		file.Close()
		t.Fatal("snapshot with multiple hard links accepted")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	snapshot, err := openDiscoveryReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	if err := os.WriteFile(replacement, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err := verifyPrivateSnapshot(snapshot); err != nil {
		t.Fatal("replacement path changed original snapshot security", err)
	}
	if file, err := OpenPrivateSnapshot(path); err == nil {
		file.Close()
		t.Fatal("replacement inherited the previous file's trust")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := verifyPrivateSnapshot(snapshot); err != nil {
		t.Fatal("unlinked snapshot could not be verified", err)
	}
	body, err := io.ReadAll(snapshot)
	if err != nil || string(body) != "original" {
		t.Fatal("opened snapshot changed", string(body), err)
	}
}

func TestSnapshotRejectsFIFOWithoutWaitingForWriter(t *testing.T) {
	if path := os.Getenv("STEERLINE_HTTPFS_FIFO_CHILD"); path != "" {
		for name, open := range map[string]func(string) (*os.File, error){
			"private": OpenPrivateSnapshot,
			"regular": OpenRegularSnapshot,
		} {
			file, err := open(path)
			if err == nil {
				file.Close()
				t.Errorf("%s snapshot accepted FIFO", name)
			}
		}
		return
	}
	path := filepath.Join(t.TempDir(), "snapshot.fifo")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestSnapshotRejectsFIFOWithoutWaitingForWriter$", "-test.count=1")
	cmd.Env = append(os.Environ(), "STEERLINE_HTTPFS_FIFO_CHILD="+path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("snapshot must reject FIFO without a writer: %v (%v)\n%s", err, ctx.Err(), output)
	}
}
