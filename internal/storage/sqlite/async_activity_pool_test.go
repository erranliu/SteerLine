package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

func TestAsyncActivityPoolIsReadOnlyAndIndependent(t *testing.T) {
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "with spaces", "domain.db"))
	require(t, err)
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require(t, s.RegisterPreparation(ctx, "p", "request", pm.GenerationBatch{ID: "batch", Status: "queued"}))
	// Hold the writer's sole connection. Activity reads must still complete.
	tx, err := s.db.BeginTx(ctx, nil)
	require(t, err)
	rows, _, readErr := s.ReadAsyncActivityFacts(ctx, "p", ports.AsyncActivityRead{Source: "test_preparation", Filter: "ongoing", Limit: 50})
	require(t, tx.Rollback())
	require(t, readErr)
	if len(rows) != 1 {
		t.Fatalf("observation shared the writer connection: %+v", rows)
	}
	pool, err := s.asyncActivityPool()
	require(t, err)
	first, err := pool.Conn(ctx)
	require(t, err)
	second, err := pool.Conn(ctx)
	require(t, err)
	if pool.Stats().MaxOpenConnections != 2 {
		t.Fatal("unbounded observation pool")
	}
	var queryOnly int
	require(t, first.QueryRowContext(ctx, "PRAGMA query_only").Scan(&queryOnly))
	if queryOnly != 1 {
		t.Fatal("query_only not set on observation connection")
	}
	_, err = first.ExecContext(ctx, "PRAGMA query_only=OFF")
	require(t, err)
	if _, err = first.ExecContext(ctx, "INSERT INTO source_observations VALUES('p','bad','bad',1,'','')"); err == nil {
		t.Fatal("mode=ro allowed a write")
	}
	// Saturating the read pool must not hold up domain persistence.
	require(t, s.SaveMaterial(ctx, "p", pm.Material{TaskID: "1", Inputs: "input"}))
	queued, done := context.WithTimeout(ctx, 20*time.Millisecond)
	_, _, err = s.ReadAsyncActivityFacts(queued, "p", ports.AsyncActivityRead{Source: "test_preparation", Limit: 50})
	done()
	if err == nil {
		t.Fatal("pool wait did not honor query context")
	}
	require(t, first.Close())
	require(t, second.Close())
	// Fail the view's pool. Business saves still work; reads cannot fall back.
	require(t, pool.Close())
	require(t, s.SaveMaterial(ctx, "p", pm.Material{TaskID: "2", Inputs: "another"}))
	if _, _, err = s.ReadAsyncActivityFacts(ctx, "p", ports.AsyncActivityRead{Source: "test_preparation", Limit: 50}); err == nil {
		t.Fatal("observation fell back to writer pool")
	}
}

func TestAsyncActivityPoolCancelsSQLAndClosesWithOwner(t *testing.T) {
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "domain.db"))
	require(t, err)
	pool, err := s.asyncActivityPool()
	require(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	var sum int64
	err = pool.QueryRowContext(ctx, `WITH RECURSIVE n(x) AS (SELECT 0 UNION ALL SELECT x+1 FROM n WHERE x<100000000) SELECT SUM(x) FROM n`).Scan(&sum)
	if err == nil || ctx.Err() == nil {
		t.Fatalf("expensive SQL ignored cancellation: %v", err)
	}
	require(t, s.Close())
	if err = pool.PingContext(context.Background()); err == nil {
		t.Fatal("observation pool outlived its owner")
	}
}
