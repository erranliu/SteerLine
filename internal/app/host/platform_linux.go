//go:build linux

package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/erranliu/SteerLine/internal/hostcontract"
)

func serviceConsoleWindow() uint64 { return 0 }

func ServiceWindowContext(context.Context, string) (context.Context, context.CancelFunc, error) {
	return nil, nil, errors.New("service-window requires Windows")
}

func canonical(s string) string { return filepath.Clean(s) }

func readSnapshot(path string) ([]byte, error) { return os.ReadFile(path) }

func replaceFile(from, to string) error { return os.Rename(from, to) }

type RepoLock struct{ f *os.File }

func LockRepository(identity string) (*RepoLock, error) {
	l, e := lockPath(filepath.Join(identity, "steerline.lock"))
	if e != nil {
		return nil, fmt.Errorf("repository already locked: %w", e)
	}
	return l, nil
}

func (l *RepoLock) Close() { l.f.Close() }

func lockPath(path string) (*RepoLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, errLockBusy
		}
		return nil, err
	}
	return &RepoLock{f: f}, nil
}

func tryExistingLock(path string) (*RepoLock, error) { return lockPath(path) }

func probeLockFile(path string) (bool, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return false, nil
}

func startDetachedProcess(cmd *exec.Cmd, facts hostcontract.ProcessFactsPort) (hostcontract.ProcessIdentity, error) {
	// A separate session detaches the service from the caller's terminal and
	// process group. Pdeathsig is deliberately unset for this lifecycle.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return hostcontract.ProcessIdentity{}, err
	}
	id, err := facts.IdentityForPID(cmd.Process.Pid)
	if err == nil {
		var session int
		session, err = facts.SessionID(cmd.Process.Pid)
		if err == nil && session != cmd.Process.Pid {
			err = fmt.Errorf("detached service session verification failed: session=%d pid=%d", session, cmd.Process.Pid)
		}
	}
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return hostcontract.ProcessIdentity{}, fmt.Errorf("cannot detach service from caller session: %w", err)
	}
	return id, nil
}

func currentJobDiagnostics() (bool, uint32, error) { return false, 0, nil }

func startDetachedProductProcess(cmd *exec.Cmd, facts hostcontract.ProcessFactsPort) (hostcontract.ProcessIdentity, error) {
	return startDetachedProcess(cmd, facts)
}
