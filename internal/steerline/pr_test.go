package steerline

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRuntimeCorruptionNotOverwritten(t *testing.T) {
	f := newFixture(t)
	s := f.service(t)
	require(t, s.Close())
	db, err := OpenSQLiteStore(filepath.Join(f.c.RunDir, "steerline.db"))
	require(t, err)
	defer db.Close()
	require(t, db.transactSQLite(context.Background(), func(tx *sqliteTx) error {
		return tx.PutProjection(f.c.Identity(), 99, []byte("truncated{"))
	}))
	if _, e := newTestService(f.c); e == nil {
		t.Fatal("corrupt snapshot accepted")
	}
	b, _, err := db.LoadProjection(context.Background(), f.c.Identity())
	require(t, err)
	if string(b) != "truncated{" {
		t.Fatal("corrupt evidence overwritten")
	}
}
