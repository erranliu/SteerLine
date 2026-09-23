//go:build linux

package steerline

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func canonical(s string) string                { return filepath.Clean(s) }
func readSnapshot(path string) ([]byte, error) { return os.ReadFile(path) }
func replaceFile(from, to string) error        { return os.Rename(from, to) }

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

type childProcess struct{ identity ProcessIdentity }

func (p *childProcess) Identity() ProcessIdentity { return p.identity }
func (p *childProcess) Kill()                     { syscall.Kill(-p.identity.PID, syscall.SIGKILL) }
func (p *childProcess) Close()                    { p.Kill() }
func startProcess(cmd *exec.Cmd) (*childProcess, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	if e := cmd.Start(); e != nil {
		return nil, e
	}
	id, e := linuxIdentity(cmd.Process.Pid)
	if e != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return nil, e
	}
	return &childProcess{identity: id}, nil
}
func startDetachedProcess(cmd *exec.Cmd) (ProcessIdentity, error) {
	// A separate session detaches the service from the caller's terminal and
	// process group. Pdeathsig is deliberately unset for this lifecycle.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return ProcessIdentity{}, err
	}
	id, err := linuxIdentity(cmd.Process.Pid)
	if err == nil {
		var session int
		session, err = linuxSessionID(cmd.Process.Pid)
		if err == nil && session != cmd.Process.Pid {
			err = fmt.Errorf("detached service session verification failed: session=%d pid=%d", session, cmd.Process.Pid)
		}
	}
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return ProcessIdentity{}, fmt.Errorf("cannot detach service from caller session: %w", err)
	}
	return id, nil
}

func currentJobDiagnostics() (bool, uint32, error) { return false, 0, nil }

func linuxProcFields(pid int) ([]string, error) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return nil, err
	}
	text := string(b)
	end := strings.LastIndex(text, ")")
	if end < 0 || end+1 >= len(text) {
		return nil, fmt.Errorf("invalid proc stat")
	}
	fields := strings.Fields(text[end+1:])
	if len(fields) < 20 {
		return nil, fmt.Errorf("invalid proc stat")
	}
	return fields, nil
}

func linuxSessionID(pid int) (int, error) {
	fields, err := linuxProcFields(pid)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(fields[3])
}
func linuxIdentity(pid int) (ProcessIdentity, error) {
	base := "/proc/" + strconv.Itoa(pid)
	fields, e := linuxProcFields(pid)
	if e != nil {
		return ProcessIdentity{}, e
	}
	if fields[0] == "Z" {
		return ProcessIdentity{}, os.ErrNotExist
	}
	exe, e := os.Readlink(base + "/exe")
	if e != nil {
		return ProcessIdentity{}, e
	}
	boot, e := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if e != nil {
		return ProcessIdentity{}, e
	}
	return ProcessIdentity{Kind: "os", PID: pid, Created: strings.TrimSpace(string(boot)) + ":" + fields[19], Executable: exe}, nil
}
func currentProcessIdentity() (*ProcessIdentity, error) {
	id, err := linuxIdentity(os.Getpid())
	if err != nil {
		return nil, err
	}
	return &id, nil
}
func inspectProcess(old *ProcessIdentity) string {
	probe := probeProcess(old)
	switch probe.State {
	case ProcessAbsent:
		return "process absent"
	case ProcessMatches:
		return "matching process still exists"
	case ProcessReused:
		return "PID belongs to a different process"
	default:
		return "process identity unavailable: " + probe.Reason
	}
}

func probeProcess(old *ProcessIdentity) ProcessProbe {
	if old == nil || old.PID <= 0 || old.Kind != "os" {
		return ProcessProbe{State: ProcessUnknown, Reason: "process identity is missing or unsupported"}
	}
	id, err := linuxIdentity(old.PID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ProcessProbe{State: ProcessAbsent, Reason: "process does not exist"}
		}
		return ProcessProbe{State: ProcessUnknown, Reason: err.Error()}
	}
	if id == *old {
		return ProcessProbe{State: ProcessMatches, Reason: "process creation identity and executable match"}
	}
	return ProcessProbe{State: ProcessReused, Reason: "PID belongs to a different process identity"}
}
