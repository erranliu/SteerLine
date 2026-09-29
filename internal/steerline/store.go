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

// Domain rows own business state. The projection keeps only service and worker
// session details that are not represented by those rows.
func runtimeCheckpoint(v Runtime) Runtime {
	v.Tasks, v.WorkflowRuns, v.Executions, v.Attempts, v.Decisions, v.Operations = nil, nil, nil, nil, nil, nil
	return v
}

func hydrateRuntime(v *Runtime, snapshot DomainSnapshot) {
	v.Tasks, v.WorkflowRuns, v.Executions = snapshot.Tasks, snapshot.WorkflowRuns, snapshot.Executions
	v.Attempts, v.Decisions, v.Operations = snapshot.Attempts, snapshot.Decisions, snapshot.Operations
}

// Store serializes publication with heartbeat. SQLite is the only durable
// runtime store.
type Store struct {
	mu      sync.Mutex
	Domain  *SQLiteStore
	Project string
	failure error
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
		persisted, err := s.Domain.SaveRuntime(context.Background(), value)
		if err != nil {
			s.failure = fmt.Errorf("critical domain persistence: %w", err)
			return s.failure
		}
		value = persisted
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
	if err := s.Domain.Transact(context.Background(), func(tx DomainTx) error {
		return tx.(*sqliteTx).heartbeatProjection(s.Project)
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
	payload, _, err := db.LoadProjection(context.Background(), c.Identity())
	if err != nil || len(payload) == 0 {
		return nil, err
	}
	var state Runtime
	if err = decode(payload, &state); err != nil {
		return nil, err
	}
	if state.SchemaVersion != WorkerFlowRuntimeSchema {
		return nil, fmt.Errorf("unsupported runtime schema %d", state.SchemaVersion)
	}
	return &state, nil
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
	var state Runtime
	if err = decode(payload, &state); err != nil {
		return nil, err
	}
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
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	hydrateRuntime(&state, snapshot)
	return &state, nil
}
