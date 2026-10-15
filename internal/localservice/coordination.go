package localservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	fs "github.com/erranliu/SteerLine/internal/product/httpfs"
)

// Coordinate uses a lease distinct from resident service ownership. All time
// spent waiting is charged to the caller's one operation deadline.
func Coordinate(ctx context.Context, home string) (func(), error) {
	dir := filepath.Join(home, "startup-coordination")
	if err := fs.EnsurePrivateDirectory(dir); err != nil {
		return nil, err
	}
	tick := time.NewTicker(40 * time.Millisecond)
	defer tick.Stop()
	for {
		l, err := lockLease(dir)
		if err == nil {
			return l.Close, nil
		}
		if !leaseBusy(err) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("startup coordination lock: %w", ctx.Err())
		case <-tick.C:
		}
	}
}

func LeaseAvailable(dir string) (bool, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return true, nil
	} else if err != nil {
		return false, err
	}
	l, err := lockLease(dir)
	if err != nil {
		if leaseBusy(err) {
			return false, nil
		}
		return false, err
	}
	l.Close()
	return true, nil
}

func ReadPrivateJSON(path string, target any, limit int64) error {
	if err := fs.VerifyPrivateFile(path); err != nil {
		return err
	}
	f, err := fs.OpenPrivateSnapshot(path)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > limit {
		return errors.New("private lifecycle record exceeds bound")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("one lifecycle record required")
	}
	return nil
}

// ReadInstance survives endpoint removal. Exact process exit is probed by the
// caller; absent discovery alone is never treated as an absent service.
func ReadInstance(home, dir, protocol string) (contract.Identity, error) {
	var identity contract.Identity
	err := ReadPrivateJSON(filepath.Join(dir, "instance.json"), &identity, 64<<10)
	if err != nil {
		return identity, err
	}
	id, err := HomeID(home)
	if err != nil {
		return identity, err
	}
	if identity.HomeID != id || identity.Protocol != protocol || len(identity.InstanceID) != 64 || !Address(identity.Address) {
		return identity, errors.New("lifecycle record home/protocol/instance differs")
	}
	return identity, nil
}

func WaitExited(ctx context.Context, dir string, id contract.Identity) error {
	var lastProbe error
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		exited, err := httpclient.ProcessExited(id.Process)
		if err != nil {
			lastProbe = err
		}
		if err == nil && exited {
			free, err := LeaseAvailable(dir)
			if err != nil {
				return err
			}
			if free {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("exact process exit and lease release unverified: %w", errors.Join(ctx.Err(), lastProbe))
		case <-tick.C:
		}
	}
}
