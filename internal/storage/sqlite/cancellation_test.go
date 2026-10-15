package sqlite

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	sqlitedriver "modernc.org/sqlite"
)

var cancellationProbeID atomic.Uint64

// A canceled query can return no Rows to its caller while the driver still
// owns a prepared statement. Checkpointing on the same connection detects that
// leak without relying on Windows directory cleanup or another test's timing.
func TestCanceledSQLiteQueryReleasesStatement(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	name := fmt.Sprintf("steerline_cancel_probe_%d", cancellationProbeID.Add(1))
	require(t, sqlitedriver.RegisterDeterministicScalarFunction(name, 0,
		func(*sqlitedriver.FunctionContext, []driver.Value) (driver.Value, error) {
			close(entered)
			<-ctx.Done()
			// Keep sqlite3_step active while the driver's interrupt goroutine
			// observes cancellation, then let the SQL function finish normally.
			time.Sleep(20 * time.Millisecond)
			return int64(42), nil
		}))
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	require(t, err)
	defer func() { require(t, store.Close()) }()
	conn, err := store.db.Conn(context.Background())
	require(t, err)
	defer func() { require(t, conn.Close()) }()
	done := make(chan error, 1)
	go func() {
		rows, queryErr := conn.QueryContext(ctx, "SELECT "+name+"()")
		if rows != nil {
			queryErr = errors.Join(queryErr, rows.Close())
		}
		done <- queryErr
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("query did not enter the controlled SQL function")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("query cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled query did not return")
	}
	if _, err := conn.ExecContext(context.Background(), "PRAGMA wal_checkpoint"); err != nil {
		t.Fatalf("canceled query retained a statement or read transaction: %v", err)
	}
}
