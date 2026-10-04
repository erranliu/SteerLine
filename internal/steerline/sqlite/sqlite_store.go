package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	_ "modernc.org/sqlite"

	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

type SQLiteStore struct {
	Reader
	db         *sql.DB
	readOnly   bool
	path       string
	createdNew bool
}

func OpenSQLiteStore(path string) (*SQLiteStore, error) {
	if path == "" {
		return nil, errors.New("sqlite path is required")
	}
	createNew := false
	if _, err := os.Stat(path); err == nil {
		if err = validateExistingDatabase(path); err != nil {
			return nil, err
		}
	} else if os.IsNotExist(err) {
		createNew = true
	} else {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open SQLite store: %w", err)
	}
	db.SetMaxOpenConns(1)
	// Closing idle connections avoids keeping runtime directories undeletable on
	// Windows when a caller forgets to close an otherwise idle Service.
	db.SetMaxIdleConns(0)
	store := &SQLiteStore{db: db, Reader: NewReader(db, nil), path: path, createdNew: createNew}
	if err := store.ensureSchema(context.Background(), createNew); err != nil {
		db.Close()
		store.abortNewDatabase()
		return nil, err
	}
	return store, nil
}

func OpenSQLiteReader(path string) (*SQLiteStore, error) {
	reader, err := openSQLiteInspector(path)
	if err != nil {
		return nil, err
	}
	if err = validateDatabaseSchema(context.Background(), reader.db); err != nil {
		_ = reader.Close()
		return nil, err
	}
	return reader, nil
}

// openSQLiteInspector reads only metadata needed to reject an unsupported file.
// Callers must validate the current schema before exposing business data.
func openSQLiteInspector(path string) (*SQLiteStore, error) {
	if path == "" {
		return nil, errors.New("sqlite path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	uriPath := filepath.ToSlash(abs)
	if runtime.GOOS == "windows" {
		uriPath = "/" + uriPath
	}
	u := url.URL{Scheme: "file", Path: uriPath}
	db, err := sql.Open("sqlite", u.String()+"?mode=ro&_pragma=busy_timeout(5000)&_pragma=query_only(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	if err = db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &SQLiteStore{db: db, readOnly: true, Reader: NewReader(db, nil)}, nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

// Only a file created by this resource can be removed after failed initialization.
// Existing or unsupported databases are always retained for recovery.
func (s *SQLiteStore) abortNewDatabase() {
	if !s.createdNew {
		return
	}
	_ = s.db.Close()
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(s.path + suffix)
	}
	s.createdNew = false
}

// transactSQLite is private to the unique SQLite executor.
func (s *SQLiteStore) transactSQLite(ctx context.Context, fn func(*sqliteTx) error) (retErr error) {
	defer func() { retErr = storageFailure(retErr, storageports.StorageIO) }()
	if s.readOnly {
		return errors.New("SQLite reader is read-only")
	}
	if fn == nil {
		return errors.New("transaction callback is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(&sqliteTx{ctx: ctx, tx: tx}); err != nil {
		return err
	}
	if err = validateNativeTaskStructure(ctx, tx); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE database_state SET revision=revision+1 WHERE singleton=1`); err != nil {
		return err
	}
	return storageFailure(tx.Commit(), storageports.StorageCommitFailed)
}
