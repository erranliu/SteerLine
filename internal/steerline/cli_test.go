package steerline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	digest := sha256.Sum256(data)
	f.c.configDigest = hex.EncodeToString(digest[:])
	log, e := os.Create(filepath.Join(f.c.RunDir, "cli.log"))
	require(t, e)
	defer log.Close()
	cmd := exec.Command(exe, "run", "-config", configPath, "-console", "json")
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
	readyDeadline := time.Now().Add(15 * time.Second)
	for {
		if rt, err := ControlSnapshot(f.c); err == nil {
			if rt.Control.DispatchMode != "auto" {
				t.Fatalf("default run mode=%q", rt.Control.DispatchMode)
			}
			break
		}
		if time.Now().After(readyDeadline) {
			connection := ConnectService(context.Background(), f.c)
			b, _ := os.ReadFile(log.Name())
			t.Fatalf("CLI runtime did not become control-ready: state=%s diagnostic=%q runtime=%+v log=%s", connection.State, connection.Diagnostic, connection.Snapshot.Runtime, b)
		}
		time.Sleep(25 * time.Millisecond)
	}
	var taskSnapshot TaskSnapshot
	tasksDeadline := time.Now().Add(15 * time.Second)
	for {
		tasksOut, taskErr := exec.Command(exe, "tasks", "-config", configPath).CombinedOutput()
		if taskErr == nil && json.Unmarshal(tasksOut, &taskSnapshot) == nil && taskSnapshot.InstanceID != "" {
			break
		}
		if time.Now().After(tasksDeadline) {
			t.Fatalf("task candidate CLI: %v %s", taskErr, tasksOut)
		}
		time.Sleep(25 * time.Millisecond)
	}
	controlOut, e := exec.Command(exe, "control", "-config", configPath, "-action", "set_dispatch_mode", "-mode", "manual", "-wait").CombinedOutput()
	if e != nil {
		t.Fatalf("control CLI submit/wait: %v %s", e, controlOut)
	}
	fields := strings.Fields(string(controlOut))
	var request string
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "request_id" {
			request = fields[i+1]
			break
		}
	}
	if request == "" {
		t.Fatalf("control CLI omitted request ID: %s", controlOut)
	}
	queryOut, e := exec.Command(exe, "control", "-config", configPath, "-request-id", request).CombinedOutput()
	if e != nil {
		t.Fatalf("control CLI receipt query: %v %s", e, queryOut)
	}
	var receipt ControlReceipt
	if e = json.Unmarshal(queryOut, &receipt); e != nil || receipt.Status != "applied" || receipt.RequestID != request {
		t.Fatalf("CLI receipt query returned %s (%v)", queryOut, e)
	}
	if rt, err := ControlSnapshot(f.c); err != nil || rt.Control.DispatchMode != "manual" {
		t.Fatalf("CLI did not apply manual mode: runtime=%+v err=%v", rt, err)
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
	stopOut, e := exec.Command(exe, "control", "-config", configPath, "-action", "stop_service", "-wait").CombinedOutput()
	if e != nil {
		t.Fatalf("control stop: %v %s", e, stopOut)
	}
	stopFields := strings.Fields(string(stopOut))
	stopID := ""
	for i := 0; i+1 < len(stopFields); i++ {
		if stopFields[i] == "request_id" {
			stopID = stopFields[i+1]
			break
		}
	}
	if stopID == "" {
		t.Fatalf("control stop omitted request ID: %s", stopOut)
	}
	stopQuery, e := exec.Command(exe, "control", "-config", configPath, "-request-id", stopID).CombinedOutput()
	if e != nil {
		t.Fatalf("control stop receipt query: %v %s", e, stopQuery)
	}
	var stopReceipt ControlReceipt
	if e = json.Unmarshal(stopQuery, &stopReceipt); e != nil || stopReceipt.Status != "applied" {
		t.Fatalf("control stop receipt=%s err=%v", stopQuery, e)
	}
	select {
	case e := <-done:
		require(t, e)
	case <-time.After(10 * time.Second):
		t.Fatal("control stop did not exit the native CLI")
	}
	jsonOutput, e := os.ReadFile(log.Name())
	require(t, e)
	for _, line := range strings.Split(strings.TrimSpace(string(jsonOutput)), "\n") {
		if !json.Valid([]byte(line)) {
			t.Fatalf("JSON console emitted a non-JSON line: %q", line)
		}
		event, err := DecodeActivityLine([]byte(line))
		if err != nil || event.V != ActivityVersion || event.TS.IsZero() {
			t.Fatalf("JSON console emitted an invalid activity record: %s (%v)", line, err)
		}
	}
	secondCmd := exec.Command(exe, "run", "-config", configPath)
	secondCmd.Stdout, secondCmd.Stderr = log, log
	require(t, secondCmd.Start())
	secondDone := make(chan error, 1)
	go func() { secondDone <- secondCmd.Wait() }()
	defer secondCmd.Process.Kill()
	secondDeadline := time.Now().Add(15 * time.Second)
	for {
		rt, err := ControlSnapshot(f.c)
		if err == nil && rt.Service.InstanceID != stopReceipt.InstanceID {
			break
		}
		if time.Now().After(secondDeadline) {
			t.Fatal("second service instance did not become ready")
		}
		time.Sleep(25 * time.Millisecond)
	}
	out, e = exec.Command(exe, "stop", "-config", configPath).CombinedOutput()
	if e != nil {
		t.Fatalf("legacy stop: %v %s", e, out)
	}
	select {
	case e := <-secondDone:
		require(t, e)
	case <-time.After(10 * time.Second):
		t.Fatal("legacy stop did not exit the native CLI")
	}
	r, e := LoadRuntime(filepath.Join(f.c.RunDir, f.c.RuntimeFile))
	require(t, e)
	if r.Service.Status != "stopped" {
		t.Fatal("CLI did not publish final status")
	}
}
