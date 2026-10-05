package host

import (
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"github.com/erranliu/SteerLine/internal/execution"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
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

// LoadDatabaseRuntime delegates to the application-owned execution snapshot reader.
func LoadDatabaseRuntime(c execution.Config) (*reconstructiontypes.Runtime, error) {
	return readConfiguredRuntime(context.Background(), c)
}

func readConfiguredRuntime(ctx context.Context, c execution.Config) (*reconstructiontypes.Runtime, error) {
	if err := c.RequireNativeTaskAuthority(); err != nil {
		return nil, err
	}
	if c.Identity() == "" {
		if err := c.ResolveIdentity(context.Background()); err != nil {
			return nil, err
		}
	}
	if c.ReadRuntime == nil {
		return nil, errors.New("application runtime read port is required")
	}
	return c.ReadRuntime(ctx, c)
}

func loadObservedRuntime(c execution.Config) (*reconstructiontypes.Runtime, error) {
	if c.Identity() == "" {
		if err := c.ResolveIdentity(context.Background()); err != nil {
			return nil, err
		}
	}
	if c.ReadRuntime == nil {
		return nil, errors.New("application runtime read port is required")
	}
	value, err := c.ReadRuntime(context.Background(), c)
	if errors.Is(err, storageports.ErrNotFound) {
		return nil, nil
	}
	return value, err
}
