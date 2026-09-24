package steerline

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	f, e := os.CreateTemp(filepath.Dir(path), ".steerline-*.tmp")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(data); e == nil {
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

// The store owns an independent snapshot. Heartbeats never race with scheduler mutations.
type Store struct {
	mu                sync.Mutex
	Path              string
	Domain            DomainStore
	Project           string
	last              Runtime
	ready             bool
	failure           error
	projectionFailure error
}

func (s *Store) write(v Runtime) error {
	v.UpdatedAt = now()
	v.HeartbeatAt = v.UpdatedAt
	b, e := json.MarshalIndent(v, "", "  ")
	if e == nil {
		e = AtomicWrite(s.Path, append(b, '\n'))
	}
	if e != nil {
		s.failure = fmt.Errorf("critical runtime persistence: %w", e)
		return s.failure
	}
	// Detach pointers and slices from the scheduler's mutable state.
	if e = json.Unmarshal(b, &s.last); e != nil {
		return e
	}
	s.ready = true
	return nil
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
	if s.Domain != nil {
		writer, ok := s.Domain.(interface {
			SaveRuntime(context.Context, Runtime) (Runtime, error)
		})
		if !ok {
			return errors.New("configured domain store cannot persist runtime state")
		}
		persisted, err := writer.SaveRuntime(context.Background(), value)
		if err != nil {
			s.failure = fmt.Errorf("critical domain persistence: %w", err)
			return s.failure
		}
		value = persisted
	}
	value.UpdatedAt = now()
	value.HeartbeatAt = value.UpdatedAt
	*v = value
	b, e := json.MarshalIndent(value, "", "  ")
	if e == nil {
		e = AtomicWrite(s.Path, append(b, '\n'))
	}
	if e != nil {
		s.projectionFailure = fmt.Errorf("runtime projection is stale: %w", e)
		// SQLite has already committed the authoritative state. The runtime
		// projection is rebuildable and must never roll that transaction back.
		if s.Domain != nil {
			value.ProjectionError = s.projectionFailure.Error()
			*v = value
			s.last = value
			s.ready = true
			return nil
		}
		s.failure = fmt.Errorf("critical runtime persistence: %w", e)
		return s.failure
	}
	if e = json.Unmarshal(b, &s.last); e != nil {
		return e
	}
	s.projectionFailure = nil
	s.ready = true
	return nil
}
func (s *Store) Heartbeat() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure != nil {
		return s.failure
	}
	if !s.ready {
		return nil
	}
	if s.Domain != nil {
		v := s.last
		v.UpdatedAt = now()
		v.HeartbeatAt = v.UpdatedAt
		v.ProjectionError = ""
		b, marshalErr := json.MarshalIndent(v, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		databaseErr := s.Domain.Transact(context.Background(), func(tx DomainTx) error { return tx.PutProjection(s.Project, v.Revision, b) })
		if databaseErr != nil {
			s.projectionFailure = fmt.Errorf("database projection heartbeat failed: %w", databaseErr)
		}
		if err := AtomicWrite(s.Path, append(b, '\n')); err != nil {
			s.projectionFailure = fmt.Errorf("runtime projection is stale: %w", err)
			return nil
		}
		if err := json.Unmarshal(b, &s.last); err != nil {
			return err
		}
		if databaseErr == nil {
			s.projectionFailure = nil
		}
		return nil
	}
	return s.write(s.last)
}
func (s *Store) Err() error           { s.mu.Lock(); defer s.mu.Unlock(); return s.failure }
func (s *Store) ProjectionErr() error { s.mu.Lock(); defer s.mu.Unlock(); return s.projectionFailure }
func LoadRuntime(path string) (*Runtime, error) {
	b, e := readSnapshot(path)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var v Runtime
	if e = decode(b, &v); e != nil {
		return nil, fmt.Errorf("invalid runtime; preserve it for manual recovery: %w", e)
	}
	if v.SchemaVersion != 5 {
		return nil, fmt.Errorf("unsupported runtime schema %d", v.SchemaVersion)
	}
	return &v, nil
}
