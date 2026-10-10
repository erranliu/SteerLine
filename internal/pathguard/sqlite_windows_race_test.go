//go:build windows

package pathguard_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/erranliu/SteerLine/internal/pathguard"
	_ "modernc.org/sqlite"
)

// Exercise SQLite's last-connection SHM create/unmap/delete path while native
// admission checks the same database and sidecars for shared hard links.
func TestRejectSharedSQLiteFilesDuringConcurrentSidecarLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	initializer, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	initializer.SetMaxOpenConns(1)
	initializer.SetMaxIdleConns(0)
	if _, err = initializer.Exec("CREATE TABLE IF NOT EXISTS race_probe (id INTEGER PRIMARY KEY, value TEXT)"); err != nil {
		t.Fatal(err)
	}
	if err = initializer.Close(); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var first error
	var failures int
	var writers sync.WaitGroup
	for writer := 0; writer < 4; writer++ {
		writers.Add(1)
		go func(writerID int) {
			defer writers.Done()
			for n := 0; n < 200; n++ {
				db, openErr := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
				if openErr != nil {
					mu.Lock()
					failures++
					if first == nil {
						first = openErr
					}
					mu.Unlock()
					return
				}
				db.SetMaxOpenConns(1)
				db.SetMaxIdleConns(0)
				_, writeErr := db.Exec("INSERT INTO race_probe(value) VALUES(?)", fmt.Sprintf("%d-%d", writerID, n))
				closeErr := db.Close()
				if writeErr != nil || closeErr != nil {
					mu.Lock()
					failures++
					if first == nil {
						first = fmt.Errorf("sqlite write=%v close=%v", writeErr, closeErr)
					}
					mu.Unlock()
					return
				}
			}
		}(writer)
	}
	var inspections sync.WaitGroup
	for checker := 0; checker < 4; checker++ {
		inspections.Add(1)
		go func() {
			defer inspections.Done()
			for n := 0; n < 1200; n++ {
				if checkErr := pathguard.RejectSharedSQLiteFiles(path); checkErr != nil {
					mu.Lock()
					failures++
					if first == nil {
						first = checkErr
					}
					mu.Unlock()
				}
			}
		}()
	}
	writers.Wait()
	inspections.Wait()
	mu.Lock()
	defer mu.Unlock()
	if failures != 0 {
		t.Fatalf("SQLite sidecar lifecycle caused %d admission inspection failures; first: %v", failures, first)
	}
}
