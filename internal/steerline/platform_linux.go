//go:build linux

package steerline

import (
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
	f, e := os.OpenFile(filepath.Join(identity, "steerline.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, fmt.Errorf("repository already locked: %w", e)
	}
	return &RepoLock{f}, nil
}
func (l *RepoLock) Close() { l.f.Close() }

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
func linuxIdentity(pid int) (ProcessIdentity, error) {
	base := "/proc/" + strconv.Itoa(pid)
	b, e := os.ReadFile(base + "/stat")
	if e != nil {
		return ProcessIdentity{}, e
	}
	fields := strings.Fields(string(b)[strings.LastIndex(string(b), ")")+1:])
	if len(fields) < 20 {
		return ProcessIdentity{}, fmt.Errorf("invalid proc stat")
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
func inspectProcess(old *ProcessIdentity) string {
	if old == nil {
		return "identity unavailable"
	}
	id, e := linuxIdentity(old.PID)
	if e != nil {
		return "process absent or inaccessible"
	}
	if id == *old {
		return "matching process still exists"
	}
	return "PID belongs to a different process"
}
