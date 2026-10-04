//go:build linux

package process

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type childProcess struct {
	identity executionbackend.ProcessIdentity
}

func (p *childProcess) Identity() executionbackend.ProcessIdentity { return p.identity }

func (*childProcess) OwnedProcess() bool { return true }

func (p *childProcess) Kill() { syscall.Kill(-p.identity.PID, syscall.SIGKILL) }

func (p *childProcess) Close() { p.Kill() }

func StartManaged(cmd *exec.Cmd) (*childProcess, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	if e := cmd.Start(); e != nil {
		return nil, e
	}
	id, e := IdentityForPID(cmd.Process.Pid)
	if e != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return nil, e
	}
	return &childProcess{identity: id}, nil
}

func WorkerTreeAbsent(identity executionbackend.ProcessIdentity) bool {
	if InspectProcess(&identity) != "process absent" {
		return false
	}
	err := syscall.Kill(-identity.PID, 0)
	return errors.Is(err, syscall.ESRCH)
}

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

func SessionID(pid int) (int, error) {
	fields, err := linuxProcFields(pid)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(fields[3])
}

func IdentityForPID(pid int) (executionbackend.ProcessIdentity, error) {
	base := "/proc/" + strconv.Itoa(pid)
	fields, e := linuxProcFields(pid)
	if e != nil {
		return executionbackend.ProcessIdentity{}, e
	}
	if fields[0] == "Z" {
		return executionbackend.ProcessIdentity{}, os.ErrNotExist
	}
	exe, e := os.Readlink(base + "/exe")
	if e != nil {
		return executionbackend.ProcessIdentity{}, e
	}
	boot, e := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if e != nil {
		return executionbackend.ProcessIdentity{}, e
	}
	return executionbackend.ProcessIdentity{Kind: "os", PID: pid, Created: strings.TrimSpace(string(boot)) + ":" + fields[19], Executable: exe}, nil
}

func CurrentIdentity() (*executionbackend.ProcessIdentity, error) {
	id, err := IdentityForPID(os.Getpid())
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func InspectProcess(old *executionbackend.ProcessIdentity) string {
	probe := Probe(old)
	switch probe.State {
	case "absent":
		return "process absent"
	case "matches":
		return "matching process still exists"
	case "pid-reused":
		return "PID belongs to a different process"
	default:
		return "process identity unavailable: " + probe.Reason
	}
}

func Probe(old *executionbackend.ProcessIdentity) ProbeResult {
	if old == nil || old.PID <= 0 || old.Kind != "os" || old.Created == "" || old.Executable == "" {
		return ProbeResult{State: "unknown", Reason: "process identity is missing or unsupported"}
	}
	id, err := IdentityForPID(old.PID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ProbeResult{State: "absent", Reason: "process does not exist"}
		}
		return ProbeResult{State: "unknown", Reason: err.Error()}
	}
	if id == *old {
		return ProbeResult{State: "matches", Reason: "process creation identity and executable match"}
	}
	return ProbeResult{State: "pid-reused", Reason: "PID belongs to a different process identity"}
}
