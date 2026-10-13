//go:build windows

package httpfs

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	wire "github.com/erranliu/SteerLine/internal/product/httpcontract"
)

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
	time.Sleep(25 * time.Millisecond)
	previous, err := io.ReadAll(reader)
	if err != nil || !strings.Contains(string(previous), `"status":"starting"`) {
		t.Fatal("old discovery snapshot changed under its reader", err)
	}
	reader.Close()
	if err = <-done; err != nil {
		t.Fatal("brief client reader broke service readiness publication", err)
	}
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
