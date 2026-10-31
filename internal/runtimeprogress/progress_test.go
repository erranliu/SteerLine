package runtimeprogress

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func powerShell(t *testing.T) string {
	t.Helper()
	name := "pwsh"
	if runtime.GOOS == "windows" {
		name = "powershell.exe"
	}
	path, err := exec.LookPath(name)
	if err != nil {
		t.Fatal("PowerShell required for writer interoperability", err)
	}
	if runtime.GOOS == "windows" {
		var entries []string
		for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
			if _, err := os.Lstat(filepath.Join(entry, "pwsh.exe")); err != nil {
				entries = append(entries, entry)
			}
		}
		t.Setenv("PATH", strings.Join(entries, string(os.PathListSeparator)))
		if _, err := exec.LookPath("pwsh"); err == nil {
			t.Fatal("PowerShell 7 leaked into the writer test PATH")
		}
	}
	return path
}

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
	shell := powerShell(t)
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
				cmd = exec.Command(shell, "-NoProfile", "-File", filepath.Join(root, "tools", "report-progress.ps1"), "-Name", key, "-Completed")
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

func TestPowerShellProgressStrictProtocol(t *testing.T) {
	shell := powerShell(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, raw, key string
		valid          bool
	}{
		{"case-sensitive", `{"a":false,"A":false,"__type":true}`, "a", true},
		{"escapes", `{"quote\"slash\\":false,"\u4e2d\u6587":true}`, "quote\"slash\\", true},
		{"surrogate-pair", `{"\ud83d\ude00":false}`, "😀", true},
		{"date-key", `{"2026-10-31T00:00:00Z":false}`, "2026-10-31T00:00:00Z", true},
		{"empty-key", `{"":false,"a":false}`, "a", true},
		{"whitespace", " \t\r\n{\"a\" : false}\r\n", "a", true},
		{"duplicate", `{"a":false,"a":true}`, "a", false},
		{"escaped-duplicate", `{"a":false,"\u0061":true}`, "a", false},
		{"string", `{"a":"false"}`, "a", false},
		{"number", `{"a":0}`, "a", false},
		{"null-value", `{"a":null}`, "a", false},
		{"nested", `{"a":{"b":false}}`, "a", false},
		{"array", `[]`, "a", false},
		{"scalar", `true`, "a", false},
		{"null", `null`, "a", false},
		{"trailing-comma", `{"a":false,}`, "a", false},
		{"trailing-object", `{"a":false}{"b":true}`, "a", false},
		{"comment", `{/*comment*/"a":false}`, "a", false},
		{"capitalized-bool", `{"a":False}`, "a", false},
		{"control", "{\"a\n\":false}", "a", false},
		{"non-json-space", "\v{\"a\":false}", "a", false},
		{"invalid-escape", `{"\x61":false}`, "a", false},
		{"lone-high-surrogate", `{"\ud800":false}`, "a", false},
		{"lone-low-surrogate", `{"\udc00":false}`, "a", false},
		{"invalid-utf8", "{\"a\xff\":false}", "a", false},
		{"bounded-malformed-whitespace", "{" + strings.Repeat(" ", 65000) + "X", "a", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "progress.bin")
			before := []byte(tc.raw)
			if err := os.WriteFile(path, before, 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, shell, "-NoProfile", "-File", filepath.Join(root, "tools", "report-progress.ps1"), "-Name", tc.key, "-Completed")
			cmd.Env = append(os.Environ(), Environment+"="+path)
			output, err := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatal("protocol parser did not finish within its bound", ctx.Err())
			}
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v output=%s", tc.valid, err, output)
			}
			after, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !tc.valid {
				if !bytes.Equal(before, after) {
					t.Fatal("rejected input was rewritten")
				}
				return
			}
			var fields map[string]bool
			if err := json.Unmarshal(after, &fields); err != nil || !fields[tc.key] {
				t.Fatalf("updated protocol bytes: %s: %v", after, err)
			}
			if tc.name == "case-sensitive" && (fields["A"] || !fields["__type"]) {
				t.Fatal("unrelated case-sensitive fields changed")
			}
			if bytes.HasPrefix(after, []byte{0xef, 0xbb, 0xbf}) {
				t.Fatal("protocol output gained a UTF-8 BOM")
			}
		})
	}
}
