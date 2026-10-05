package testprotocol

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestTM01FixtureLockProcessDeath(t *testing.T) {
	if dir := os.Getenv("STEERLINE_TEST_LOCK_CHILD"); dir != "" {
		unlock, err := lockNativeOrcaState(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer unlock()
		if err := os.WriteFile(filepath.Join(dir, "held"), []byte("held"), 0600); err != nil {
			t.Fatal(err)
		}
		for {
			time.Sleep(time.Second)
		}
	}
	dir := t.TempDir()
	command := exec.Command(os.Args[0], "-test.run=^TestTM01FixtureLockProcessDeath$")
	command.Env = append(os.Environ(), "STEERLINE_TEST_LOCK_CHILD="+dir)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = command.Process.Kill(); _ = command.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "held")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child never acquired its fixture lock")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if unlock, err := tryNativeOrcaStateLock(filepath.Join(dir, "native-orca.lock")); err != nil || unlock != nil {
		if unlock != nil {
			unlock()
		}
		t.Fatalf("live owner did not serialize access: %v", err)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	unlock, err := lockNativeOrcaState(dir)
	if err != nil {
		t.Fatalf("terminated command stranded the fixture lock: %v", err)
	}
	unlock()
	if _, err := os.Stat(filepath.Join(dir, "native-orca.lock")); !os.IsNotExist(err) {
		t.Fatalf("lock was not cleaned: %v", err)
	}
}
