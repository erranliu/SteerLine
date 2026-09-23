package steerline

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	mu      sync.Mutex
	Path    string
	last    Runtime
	ready   bool
	failure error
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
func (s *Store) Publish(v Runtime) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure != nil {
		return s.failure
	}
	return s.write(v)
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
	return s.write(s.last)
}
func (s *Store) Err() error { s.mu.Lock(); defer s.mu.Unlock(); return s.failure }
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
	if v.SchemaVersion != 1 && v.SchemaVersion != 2 && v.SchemaVersion != 3 && v.SchemaVersion != 4 {
		return nil, fmt.Errorf("unsupported runtime schema %d", v.SchemaVersion)
	}
	return &v, nil
}
