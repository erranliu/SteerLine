package steerline

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

func newID() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func shortHash(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:12]) }
func AtomicWrite(path string, data []byte) error {
	return atomicWriteWith(path, data, func(f *os.File, data []byte) error {
		_, err := f.Write(data)
		return err
	})
}

func atomicWriteWith(path string, data []byte, write func(*os.File, []byte) error) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".steerline-*.tmp")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = write(f, data); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return replaceFile(name, path)
}

func hydrateRuntime(v *Runtime, snapshot DomainSnapshot) {
	v.Tasks, v.WorkflowRuns, v.Executions = snapshot.Tasks, snapshot.WorkflowRuns, snapshot.Executions
	v.Attempts, v.Decisions, v.Operations = snapshot.Attempts, snapshot.Decisions, snapshot.Operations
	v.Deliveries = snapshot.Deliveries
}

// Store serializes publication with heartbeat. SQLite is the only durable
// runtime store.
type Store struct {
	mu       sync.Mutex
	Domain   *SQLiteStore
	Project  string
	failure  error
	baseline map[string]string
}

func (s *Store) Publish(v *Runtime) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure != nil {
		return s.failure
	}
	if v == nil {
		return errors.New("runtime projection is required")
	}
	value := *v
	value.UpdatedAt, value.HeartbeatAt = now(), now()
	if s.Domain != nil {
		persisted, err := s.Domain.saveRuntime(context.Background(), value, s.baseline)
		if err != nil {
			s.failure = fmt.Errorf("critical domain persistence: %w", err)
			return s.failure
		}
		value = persisted
		s.baseline = domainFingerprints(value)
	}
	*v = value
	return nil
}

func (s *Store) Heartbeat() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure != nil || s.Domain == nil {
		return s.failure
	}
	if err := s.Domain.transactSQLite(context.Background(), func(tx *sqliteTx) error {
		return tx.heartbeatProjection(s.Project)
	}); err != nil {
		s.failure = fmt.Errorf("database projection heartbeat failed: %w", err)
		return s.failure
	}
	return nil
}

func (s *Store) Err() error { s.mu.Lock(); defer s.mu.Unlock(); return s.failure }

// LoadDatabaseRuntime reads one SQLite snapshot, joining the compact service
// checkpoint with the authoritative domain rows in a read transaction.
func LoadDatabaseRuntime(c Config) (*Runtime, error) {
	if err := requireNativeTaskAuthority(c); err != nil {
		return nil, err
	}
	if c.Identity() == "" {
		if err := c.ResolveIdentity(context.Background()); err != nil {
			return nil, err
		}
	}
	db, err := OpenNativeTaskReader(context.Background(), &c)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return loadRuntimeSnapshot(db, c.Identity(), true)
}

func loadObservedRuntime(c Config) (*Runtime, error) {
	if c.Identity() == "" {
		if err := c.ResolveIdentity(context.Background()); err != nil {
			return nil, err
		}
	}
	db, err := OpenSQLiteReader(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	value, err := loadRuntimeSnapshot(db, c.Identity(), false)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return value, err
}

func loadRuntimeSnapshot(db *SQLiteStore, project string, requireIdentity bool) (*Runtime, error) {
	ctx := context.Background()
	tx, err := db.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var payload []byte
	if err = tx.QueryRowContext(ctx, `SELECT payload FROM projection WHERE project_identity=?`, project).Scan(&payload); err != nil {
		return nil, err
	}
	var checkpoint ServiceCheckpoint
	if err = decode(payload, &checkpoint); err != nil {
		return nil, err
	}
	state := runtimeFromCheckpoint(checkpoint)
	if state.SchemaVersion != WorkerFlowRuntimeSchema {
		return nil, fmt.Errorf("unsupported runtime schema %d", state.SchemaVersion)
	}
	if requireIdentity && state.Repository.Identity != project {
		return nil, errors.New("database runtime repository identity mismatch")
	}
	snapshot, err := querySnapshot(ctx, tx, project)
	if err != nil {
		return nil, err
	}
	var paused int
	if err = tx.QueryRowContext(ctx, `SELECT dispatch_mode,dispatch_paused,authority_version FROM project_task_authorities WHERE project_identity=?`, project).Scan(&state.Control.DispatchMode, &paused, &state.Control.Revision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("runtime checkpoint has no task authority")
		}
		return nil, err
	}
	state.Control.DispatchPaused = paused != 0
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	hydrateRuntime(&state, snapshot)
	if err = projectRuntime(&state); err != nil {
		return nil, err
	}
	return &state, nil
}
