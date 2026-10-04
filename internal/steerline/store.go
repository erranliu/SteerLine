package steerline

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
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

// Store serializes publication with heartbeat. SQLite is the only durable
// runtime store.
type Store struct {
	mu      sync.Mutex
	Domain  persistence.WorkerMetadata
	Project string
	failure error
}

func (s *Store) commitWorker(request workerCommit) (workerCommitResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure != nil {
		return workerCommitResult{}, s.failure
	}
	if s.Domain == nil {
		return workerCommitResult{}, errors.New("worker commitment requires a durable store")
	}
	result, err := s.Domain.CommitWorker(context.Background(), request)
	var conflict *taskmodel.Error
	if err != nil && !errors.As(err, &conflict) {
		s.failure = fmt.Errorf("critical worker persistence: %w", err)
		return result, s.failure
	}
	return result, err
}

func (s *Store) Publish(checkpoint *ServiceCheckpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure != nil {
		return s.failure
	}
	if checkpoint == nil {
		return errors.New("service checkpoint is required")
	}
	value := *checkpoint
	value.UpdatedAt, value.HeartbeatAt = now(), now()
	if s.Domain != nil {
		committed, err := s.Domain.SaveCheckpoint(context.Background(), value)
		if err != nil {
			s.failure = fmt.Errorf("critical checkpoint persistence: %w", err)
			return s.failure
		}
		value = committed
	}
	*checkpoint = value
	return nil
}

func (s *Store) Heartbeat() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure != nil || s.Domain == nil {
		return s.failure
	}
	if err := s.Domain.Heartbeat(context.Background(), s.Project); err != nil {
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
	return db.LoadRuntime(c.Identity(), true)
}

func loadObservedRuntime(c Config) (*Runtime, error) {
	if c.Persistence == nil {
		return nil, errors.New("persistence factory is required")
	}
	if c.Identity() == "" {
		if err := c.ResolveIdentity(context.Background()); err != nil {
			return nil, err
		}
	}
	db, err := c.Persistence.OpenReader(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	value, err := db.LoadRuntime(c.Identity(), false)
	if errors.Is(err, storageports.ErrNotFound) {
		return nil, nil
	}
	return value, err
}
