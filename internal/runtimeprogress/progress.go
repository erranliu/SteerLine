// Package runtimeprogress updates only the runtime-injected completion map.
// Test discovery and the decision that a test passed remain with each entry point.
package runtimeprogress

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/testmanagement/model"
)

var writerMu sync.Mutex

const Environment = "STEERLINE_PROGRESS_FILE"
const MaxBytes = 64 << 10

func Update(ctx context.Context, values map[string]bool, onlyDeclared bool) error {
	writerMu.Lock()
	defer writerMu.Unlock()
	path := os.Getenv(Environment)
	if path == "" || len(values) == 0 {
		return nil
	}
	if !filepath.IsAbs(path) {
		return errors.New("progress file must be absolute")
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if err = lockFile(lock); err == nil {
			break
		}
		if !lockBusy(err) || time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(15 * time.Millisecond):
		}
	}
	defer unlockFile(lock)
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	raw, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	f.Close()
	if err != nil {
		return err
	}
	if len(raw) > MaxBytes {
		return errors.New("progress exceeds 64 KiB")
	}
	var fields map[string]json.RawMessage
	if err = model.Decode(raw, &fields); err != nil {
		return err
	}
	if fields == nil {
		return errors.New("progress requires a boolean object")
	}
	for _, v := range fields {
		if !bytes.Equal(v, []byte("true")) && !bytes.Equal(v, []byte("false")) {
			return errors.New("progress requires boolean fields")
		}
	}
	changed := false
	for key, value := range values {
		old, exists := fields[key]
		if !exists {
			if onlyDeclared {
				continue
			}
			return errors.New("progress name was not declared for this Run: " + key)
		}
		next := json.RawMessage("false")
		if value {
			next = json.RawMessage("true")
		}
		changed = changed || !bytes.Equal(old, next)
		fields[key] = next
	}
	if !changed {
		return nil
	}
	raw, err = json.Marshal(fields)
	if err != nil {
		return err
	}
	if len(raw) > MaxBytes {
		return errors.New("progress exceeds 64 KiB")
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".progress-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(raw)
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return replaceFile(tmp.Name(), path)
}
