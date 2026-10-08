//go:build windows || unix

package pathguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRejectSharedSQLiteFiles(t *testing.T) {
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		t.Run("database"+suffix, func(t *testing.T) {
			database := filepath.Join(t.TempDir(), "steerline.db")
			if err := RejectSharedSQLiteFiles(database); err != nil {
				t.Fatalf("missing state: %v", err)
			}
			path := database + suffix
			if err := os.WriteFile(path, []byte("unchanged"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := RejectSharedSQLiteFiles(database); err != nil {
				t.Fatalf("single link: %v", err)
			}
			alias := filepath.Join(t.TempDir(), "installed-state")
			if err := os.Link(path, alias); err != nil {
				t.Fatal(err)
			}
			if err := RejectSharedSQLiteFiles(database); err == nil || !strings.Contains(err.Error(), "must not be a hard link (2 links)") {
				t.Fatalf("shared %s accepted: %v", suffix, err)
			}
			if err := os.Remove(alias); err != nil {
				t.Fatal(err)
			}
			if err := RejectSharedSQLiteFiles(database); err != nil {
				t.Fatalf("restored single link: %v", err)
			}
		})
	}
}
