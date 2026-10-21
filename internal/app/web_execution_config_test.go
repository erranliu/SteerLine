package app

import (
	"context"
	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"os"
	"path/filepath"
	"testing"
)

func TestWebExecutionConfigLoadedSnapshot(t *testing.T) {
	home := t.TempDir()
	original := []byte(`{"version":3,"saved":"original"}`)
	path := filepath.Join(home, "execution.json")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	d := NewApplicationDirectory(home)
	first, err := d.Catalog(context.Background())
	if err != nil || first.LoadedExecutionDigest != cfg.Digest(original) {
		t.Fatal(first, err)
	}
	if err = os.WriteFile(path, []byte(`{"saved":"changed"}`), 0600); err != nil {
		t.Fatal(err)
	}
	next, err := d.Catalog(context.Background())
	if err != nil || next.LoadedExecutionDigest != first.LoadedExecutionDigest {
		t.Fatal("query reloaded config", next, err)
	}
	d2 := NewApplicationDirectory(home)
	fresh, err := d2.Catalog(context.Background())
	if err != nil || fresh.LoadedExecutionDigest == first.LoadedExecutionDigest {
		t.Fatal("new consumer did not read new snapshot", fresh, err)
	}
}
