package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativePreflightRejectsSharedSQLiteBeforeValidator(t *testing.T) {
	config, _ := nativeTaskFixture(t)
	if err := os.MkdirAll(config.RunDir, 0700); err != nil {
		t.Fatal(err)
	}
	database := filepath.Join(config.RunDir, "steerline.db")
	if err := os.WriteFile(database, nil, 0600); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(t.TempDir(), "installed-wal")
	if err := os.WriteFile(sentinel, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(sentinel, database+"-wal"); err != nil {
		t.Fatal(err)
	}
	input := hostConfigForTest(config)
	called := false
	input.ValidateNativeStore = func(context.Context, Config) error { called = true; return nil }
	err := PreflightNativeStatePath(input)
	if err == nil || !strings.Contains(err.Error(), "must not be a hard link") || called {
		t.Fatalf("shared state reached store validator: err=%v called=%v", err, called)
	}
}
