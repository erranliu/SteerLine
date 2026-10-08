package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeReaderRejectsSharedSQLiteBeforeOpen(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	config, _ := nativeTaskFixture(t)
	if err := os.MkdirAll(config.RunDir, 0700); err != nil {
		t.Fatal(err)
	}
	database := filepath.Join(config.RunDir, "steerline.db")
	// Any SQLite open on this file would fail as a database error. The home
	// gate must reject the shared sidecar first, without touching either file.
	if err := os.WriteFile(database, []byte("not SQLite"), 0600); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(t.TempDir(), "installed-shm")
	if err := os.WriteFile(sentinel, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(sentinel, database+"-shm"); err != nil {
		t.Fatal(err)
	}
	_, err := openBoundNativeReader(context.Background(), config, database)
	if err == nil || !strings.Contains(err.Error(), "must not be a hard link") {
		t.Fatalf("reader did not reject before SQLite open: %v", err)
	}
	if info, err := os.Stat(sentinel); err != nil || info.Size() != 0 {
		t.Fatalf("shared sentinel changed: info=%v err=%v", info, err)
	}
}
