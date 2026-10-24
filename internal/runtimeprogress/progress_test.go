package runtimeprogress

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

func TestProgressWriterProcess(t *testing.T) {
	key := os.Getenv("STEERLINE_PROGRESS_WRITER_TEST_KEY")
	if key == "" {
		return
	}
	if err := Update(context.Background(), map[string]bool{key: true}, false); err != nil {
		t.Fatal(err)
	}
}

func TestProgressConcurrentProcessesAndPowerShellUseSameLock(t *testing.T) {
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Fatal("PowerShell 7 required for writer interoperability", err)
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "progress.bin")
	t.Setenv(Environment, path)
	fields := map[string]bool{}
	for i := 0; i < 8; i++ {
		fields[fmt.Sprintf("test.%d", i)] = false
	}
	raw, _ := json.Marshal(fields)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	var wg sync.WaitGroup
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("test.%d", i)
			var cmd *exec.Cmd
			if i%2 == 0 {
				cmd = exec.Command(exe, "-test.run=^TestProgressWriterProcess$", "-test.count=1")
				cmd.Env = append(os.Environ(), "STEERLINE_PROGRESS_WRITER_TEST_KEY="+key)
			} else {
				cmd = exec.Command("pwsh", "-NoProfile", "-File", filepath.Join(root, "tools", "report-progress.ps1"), "-Name", key, "-Completed")
			}
			if output, err := cmd.CombinedOutput(); err != nil {
				failures <- fmt.Errorf("writer %d: %w: %s", i, err, output)
			}
		}(i)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for k, done := range fields {
		if !done {
			t.Error("lost concurrent field", k)
		}
	}
	if err := Update(context.Background(), map[string]bool{"test.0": false}, false); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(path)
	_ = json.Unmarshal(raw, &fields)
	if fields["test.0"] || !fields["test.1"] {
		t.Fatal("reset changed other fields")
	}
	if err := Update(context.Background(), map[string]bool{"unknown": true}, false); err == nil {
		t.Fatal("undeclared field accepted")
	}
	before := string(raw)
	if err := Update(context.Background(), map[string]bool{"unknown": true}, true); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(path)
	if string(raw) != before {
		t.Fatal("unrelated test added fields")
	}
}
