//go:build linux

package processhost

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type processHandle struct {
	identity executionbackend.ProcessIdentity
}

func (*processHandle) OwnedProcess() bool { return true }

func (handle *processHandle) Identity() executionbackend.ProcessIdentity { return handle.identity }

func (handle *processHandle) Kill() {
	if handle.identity.PID > 0 {
		_ = syscall.Kill(-handle.identity.PID, syscall.SIGKILL)
	}
}

func (handle *processHandle) Close() { handle.Kill() }

func startManaged(command *exec.Cmd) (executionbackend.StopHandle, error) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	if err := command.Start(); err != nil {
		return nil, err
	}
	identity, err := processIdentity(command.Process.Pid)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, err
	}

	return &processHandle{identity: identity}, nil
}

func inspectProcess(old *executionbackend.ProcessIdentity) string {
	if old == nil || old.PID <= 0 || old.Kind != "os" || old.Created == "" || old.Executable == "" {
		return "process identity is missing or unsupported"
	}
	identity, err := processIdentity(old.PID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "process absent"
		}
		return "process identity unavailable: " + err.Error()
	}
	if identity == *old {
		return "matching process still exists"
	}
	return "PID belongs to a different process"
}

func processIdentity(pid int) (executionbackend.ProcessIdentity, error) {
	base := "/proc/" + strconv.Itoa(pid)
	stat, err := os.ReadFile(base + "/stat")
	if err != nil {
		return executionbackend.ProcessIdentity{}, err
	}
	text := string(stat)
	end := strings.LastIndex(text, ")")
	if end < 0 || end+1 >= len(text) {
		return executionbackend.ProcessIdentity{}, fmt.Errorf("invalid process stat")
	}
	fields := strings.Fields(text[end+1:])
	if len(fields) < 20 || fields[0] == "Z" {
		return executionbackend.ProcessIdentity{}, os.ErrNotExist
	}
	executable, err := os.Readlink(filepath.Join(base, "exe"))
	if err != nil {
		return executionbackend.ProcessIdentity{}, err
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return executionbackend.ProcessIdentity{}, err
	}
	return executionbackend.ProcessIdentity{Kind: "os", PID: pid, Created: strings.TrimSpace(string(boot)) + ":" + fields[19], Executable: executable}, nil
}

func replaceFile(from, to string) error { return os.Rename(from, to) }
