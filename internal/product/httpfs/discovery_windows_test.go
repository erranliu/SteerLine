//go:build windows

package httpfs

import (
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	wire "github.com/erranliu/SteerLine/internal/product/httpcontract"
)

// Reproduce destination exclusion without the retry wrapper, retain both
// complete files on failure, then publish the same source once the holder exits.
func TestPrivateSnapshotTargetExclusionIdentifiesPublishFailure(t *testing.T) {
	root := t.TempDir()
	if err := EnsurePrivateDirectory(root); err != nil {
		t.Fatal(err)
	}
	write := func(body string) string {
		f, err := CreatePrivateTemp(root, ".discovery-")
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
	blocker, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	source := write("completed")
	err = renamePrivateSnapshot(source, path)
	if err == nil || !strings.Contains(err.Error(), "publish rename target (FileRenameInfoEx)") || (!errors.Is(err, windows.ERROR_ACCESS_DENIED) && !errors.Is(err, windows.ERROR_SHARING_VIOLATION)) {
		t.Fatalf("expected target exclusion, got %v", err)
	}
	t.Logf("held non-delete-sharing target: %v", err)
	if raw, err := os.ReadFile(path); err != nil || string(raw) != "original" {
		t.Fatal("failed rename changed previous receipt", err)
	}
	if err = blocker.Close(); err != nil {
		t.Fatal(err)
	}
	if err = renamePrivateSnapshot(source, path); err != nil {
		t.Fatal("same source failed after holder released", err)
	}
	if err = VerifyPrivateFile(path); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoveryReaderAllowsAtomicReplacement(t *testing.T) {
	root := t.TempDir()
	if err := EnsurePrivateDirectory(root); err != nil {
		t.Fatal(err)
	}
	d := wire.Discovery{Version: 1, Protocol: wire.Protocol, ProjectID: wire.ProjectID("repo"), RepositoryIdentity: "repo", InstanceID: "original", LeaseID: strings.Repeat("a", 32), Address: "127.0.0.1:1", CredentialPath: filepath.Join(root, "credentials", "original.token"), Status: "starting"}
	write := func() string {
		f, err := CreatePrivateTemp(root, ".discovery-")
		if err != nil {
			t.Fatal(err)
		}
		if err = json.NewEncoder(f).Encode(d); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
		return f.Name()
	}
	path := filepath.Join(root, "discovery.json")
	if err := os.Rename(write(), path); err != nil {
		t.Fatal(err)
	}
	reader, err := openDiscoveryReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	d.Ready, d.Status = true, "ready"
	next := write()
	done := make(chan error, 1)
	go func() { done <- ReplacePrivateFile(next, path) }()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal("snapshot reader blocked atomic readiness publication", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("replacement waited for its own snapshot reader to close")
	}
	previous, err := io.ReadAll(reader)
	if err != nil || !strings.Contains(string(previous), `"status":"starting"`) {
		t.Fatal("old discovery snapshot changed under its reader", err)
	}
	reader.Close()
	latest, err := ReadDiscoveryFile(path)
	if err != nil || !latest.Ready || latest.Status != "ready" {
		t.Fatal("new discovery snapshot not readable", latest, err)
	}
	// A persistent exclusion must remain a failure and leave the old complete
	// snapshot visible; bounded retry must not become copy/delete fallback.
	blocker, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	d.Ready, d.Status = false, "failed"
	if err = ReplacePrivateFile(write(), path); err == nil {
		blocker.Close()
		t.Fatal("persistent sharing failure was hidden")
	}
	blocker.Close()
	latest, err = ReadDiscoveryFile(path)
	if err != nil || !latest.Ready || latest.Status != "ready" {
		t.Fatal("failed replacement changed the published snapshot", latest, err)
	}
}

func TestPrivateSnapshotReplacementUsesUTF16Target(t *testing.T) {
	root := filepath.Join(t.TempDir(), "快照")
	if err := EnsurePrivateDirectory(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "回执-😀.json")
	write := func(body string) string {
		f, err := CreatePrivateTemp(root, ".discovery-")
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
	if err := ReplacePrivateFile(write("previous"), path); err != nil {
		t.Fatal(err)
	}
	reader, err := OpenPrivateSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err = ReplacePrivateFile(write("current"), path); err != nil {
		t.Fatal(err)
	}
	previous, err := io.ReadAll(reader)
	if err != nil || string(previous) != "previous" {
		t.Fatal("original UTF-16 path snapshot changed", err)
	}
	current, err := os.ReadFile(path)
	if err != nil || string(current) != "current" {
		t.Fatal("replacement did not reach the full UTF-16 target", err)
	}
	if err = VerifyPrivateFile(path); err != nil {
		t.Fatal("replacement lost the private ACL", err)
	}
}
