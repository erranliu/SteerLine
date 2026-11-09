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

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	legacyDiscard40 bool // Existing explicit discard-only reader; never a runtime compatibility path.
	Reader
	db                *sql.DB
	activityDB        *sql.DB
	activityErr       error
	readOnly          bool
	path              string
	createdNew        bool
	admission         env.AdmissionPolicy
	executionIdentity func(runnercontract.ExecutionInput) (runnercontract.ExecutionIdentity, error)
}

// BindExecutionIdentityLookup installs the application's immutable, in-memory
// Runner snapshot before publishing this writer. The lookup must perform no I/O.
func (s *SQLiteStore) BindExecutionIdentityLookup(lookup func(runnercontract.ExecutionInput) (runnercontract.ExecutionIdentity, error)) {
	s.executionIdentity = lookup
}

func (s *SQLiteStore) verifyExecutionIdentity(execution runnercontract.ExecutionInput, identity runnercontract.ExecutionIdentity) error {
	if s.executionIdentity == nil {
		return errors.New("trusted execution identity snapshot is unavailable")
	}
	trusted, err := s.executionIdentity(execution)
	if err != nil {
		return err
	}
	if runnercontract.ValidateExecutionIdentity(execution, identity) != nil || runnercontract.ConfigurationValueDigest(trusted) != runnercontract.ConfigurationValueDigest(identity) {
		return errors.New("execution identity differs from trusted Runner snapshot")
	}
	return nil
}

func OpenSQLiteStore(path string) (_ *SQLiteStore, retErr error) {
	return OpenSQLiteStoreWithAdmissionPolicy(path, env.AdmissionPolicy{})
}

// OpenSQLiteStoreWithAdmissionPolicy freezes trusted admission policy for
// this writer. Candidate payloads never authorize skipping readiness.
func OpenSQLiteStoreWithAdmissionPolicy(path string, policy env.AdmissionPolicy) (_ *SQLiteStore, retErr error) {
	defer func() { retErr = storageFailure(retErr, storageports.StorageIO) }()
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
	store := &SQLiteStore{db: db, Reader: NewReader(db, nil), path: path, createdNew: createNew, admission: policy}
	if err := store.ensureSchema(context.Background(), createNew); err != nil {
		db.Close()
		store.abortNewDatabase()
		return nil, err
	}
	// Observation owns a separate, bounded read-only pool. Its availability is
	// not an admission gate for domain work; errors surface only on activity reads.
	store.activityDB, store.activityErr = openAsyncActivityPool(path)
	return store, nil
}

func OpenSQLiteReader(path string) (_ *SQLiteStore, retErr error) {
	defer func() { retErr = storageFailure(retErr, storageports.StorageIO) }()
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

func (s *SQLiteStore) Close() error {
	var observationErr error
	if s.activityDB != nil {
		observationErr = s.activityDB.Close()
	}
	return errors.Join(observationErr, s.db.Close())
}

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
