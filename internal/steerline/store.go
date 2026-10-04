package steerline

import (
 "github.com/erranliu/SteerLine/internal/execution"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"os"
	"path/filepath"
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

// LoadDatabaseRuntime reads one SQLite snapshot, joining the compact service
// checkpoint with the authoritative domain rows in a read transaction.
func LoadDatabaseRuntime(c execution.Config) (*Runtime, error) {
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

func loadObservedRuntime(c execution.Config) (*Runtime, error) {
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
