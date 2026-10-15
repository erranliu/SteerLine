//go:build windows

package native

import (
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestFaultOwnedAppServerHelper(t *testing.T) {
	if os.Getenv("STEERRUNNER_OWNED_FAULT_HELPER") != "1" {
		return
	}
	for {
		time.Sleep(time.Second)
	}
}

func TestFaultTerminatesOnlyOriginalOwnedAppServerChild(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(executable, "-test.run=^TestFaultOwnedAppServerHelper$", "--", "app-server")
	child.Env = append(os.Environ(), "STEERRUNNER_OWNED_FAULT_HELPER=1")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer child.Process.Kill()
	parent, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(parent)
	var c, e, k, u windows.Filetime
	if err := windows.GetProcessTimes(parent, &c, &e, &k, &u); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	target := ProcessFaultTarget{ParentPID: os.Getpid(), ParentCreated: strconv.FormatUint(uint64(c.HighDateTime)<<32|uint64(c.LowDateTime), 10), Executable: executable, Workspace: filepath.Join(root, "owned"), Session: "fixture-original-session"}
	changed := target
	changed.ParentCreated = "1"
	if DisconnectCandidate(changed, root) == nil {
		t.Fatal("changed parent accepted")
	}
	if err := DisconnectCandidate(target, root); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("fault child exited successfully")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("owned child did not exit")
	}
}
