package httpclient

import (
	"context"
	"os"
	"path/filepath"
	"time"

	fs "github.com/erranliu/SteerLine/internal/product/httpfs"
)

func withRequestJournalLock(ctx context.Context, path string, action func() error) error {
	return withRequestJournalLockObserved(ctx, path, nil, action)
}

// withRequestJournalLockObserved keeps the contention callback private to the
// package. Tests use it to synchronize cancellation against a held OS lock.
func withRequestJournalLockObserved(ctx context.Context, path string, contended func(), action func() error) error {
	file, err := openPrivateRequestLock(path)
	if err != nil {
		return err
	}
	defer file.Close()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		unlock, acquired, err := tryRequestJournalLock(file)
		if err != nil {
			return err
		}
		if acquired {
			if err := ctx.Err(); err != nil {
				_ = unlock()
				return err
			}
			actionErr := action()
			unlockErr := unlock()
			if actionErr != nil {
				return actionErr
			}
			return unlockErr
		}
		if contended != nil {
			contended()
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func openPrivateRequestLock(path string) (*os.File, error) {
	if err := fs.EnsurePrivateDirectory(filepath.Dir(path)); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = fs.SecurePrivateFile(path); err != nil {
		file.Close()
		return nil, err
	}
	if err = fs.VerifyPrivateFile(path); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}
