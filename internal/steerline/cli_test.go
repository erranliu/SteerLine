package steerline

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeCLIStartSecondInstanceAndStop(t *testing.T) {
	f := newFixture(t)
	f.c.Remote = "web"
	exe := filepath.Join(t.TempDir(), "steerline.exe")
	build := exec.Command("go", "build", "-o", exe, "../../cmd/steerline")
	if b, e := build.CombinedOutput(); e != nil {
		t.Fatalf("build: %v %s", e, b)
	}
	configPath := filepath.Join(f.c.RunDir, "config.json")
	data, _ := json.Marshal(f.c)
	require(t, os.WriteFile(configPath, data, 0600))
	log, e := os.Create(filepath.Join(f.c.RunDir, "cli.log"))
	require(t, e)
	defer log.Close()
	cmd := exec.Command(exe, "run", "-config", configPath)
	cmd.Stdout = log
	cmd.Stderr = log
	require(t, cmd.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer cmd.Process.Kill()
	deadline := time.Now().Add(15 * time.Second)
	started := false
	for time.Now().Before(deadline) {
		r, e := LoadRuntime(filepath.Join(f.c.RunDir, f.c.RuntimeFile))
		if e == nil && r != nil {
			started = true
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if !started {
		b, _ := os.ReadFile(log.Name())
		t.Fatalf("CLI never started: %s", b)
	}
	other := f.c
	other.RunDir = filepath.Join(filepath.Dir(f.c.RunDir), "other runtime")
	secondConfig := filepath.Join(f.c.RunDir, "other.json")
	data, _ = json.Marshal(other)
	require(t, os.WriteFile(secondConfig, data, 0600))
	out, e := exec.Command(exe, "run", "-config", secondConfig).CombinedOutput()
	if e == nil || !strings.Contains(string(out), "repository already locked") {
		t.Fatalf("second CLI lock: %v %s", e, out)
	}
	out, e = exec.Command(exe, "stop", "-config", configPath).CombinedOutput()
	if e != nil {
		t.Fatalf("stop: %v %s", e, out)
	}
	select {
	case e := <-done:
		require(t, e)
	case <-time.After(10 * time.Second):
		t.Fatal("native CLI did not exit")
	}
	r, e := LoadRuntime(filepath.Join(f.c.RunDir, f.c.RuntimeFile))
	require(t, e)
	if r.Service.Status != "stopped" {
		t.Fatal("CLI did not publish final status")
	}
}
