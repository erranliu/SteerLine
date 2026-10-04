//go:build windows

package testprocess

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestOwnedProcessWaitsForDescendants(t *testing.T) {
	for _, action := range []string{"parent-exit", "stop"} {
		t.Run(action, func(t *testing.T) {
			dir := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.Command(os.Args[0], "-test.run=^TestOwnedProcessHelper$")
			cmd.Env = append(os.Environ(), "STEERLINE_TESTPROCESS_HELPER=parent", "STEERLINE_TESTPROCESS_DIR="+dir)
			p, err := Start(cmd)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				if err := p.Stop(stopCtx); err != nil {
					t.Error(err)
				}
			})
			var handles []windows.Handle
			defer func() {
				for _, h := range handles {
					windows.CloseHandle(h)
				}
			}()
			for i := 0; i < 8; i++ {
				pidPath := filepath.Join(dir, fmt.Sprintf("child-%d.pid", i))
				var pid int
				for pid == 0 {
					raw, err := os.ReadFile(pidPath)
					if err == nil {
						pid, _ = strconv.Atoi(string(raw))
					}
					if pid != 0 {
						break
					}
					select {
					case <-ctx.Done():
						t.Fatal("descendant did not acquire its file", ctx.Err())
					case <-p.Done:
						t.Fatal("parent exited before descendants were ready")
					case <-time.After(5 * time.Millisecond):
					}
				}
				h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
				if err != nil {
					t.Fatal(err)
				}
				handles = append(handles, h)
			}
			if action == "parent-exit" {
				if err := os.WriteFile(filepath.Join(dir, "exit-parent"), nil, 0600); err != nil {
					t.Fatal(err)
				}
				err = p.Wait(ctx)
			} else {
				err = p.Stop(ctx)
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, h := range handles {
				status, err := windows.WaitForSingleObject(h, 0)
				if err != nil || status != windows.WAIT_OBJECT_0 {
					t.Fatalf("Done published before descendant exit: status=%d err=%v", status, err)
				}
			}
			// Each descendant held an exclusive file handle. Cleanup must succeed
			// immediately, without retrying deletion to hide an ownership race.
			if err := os.RemoveAll(dir); err != nil {
				t.Fatal("descendant files remained locked after Done", err)
			}
		})
	}
}

func TestOwnedProcessHelper(t *testing.T) {
	mode := os.Getenv("STEERLINE_TESTPROCESS_HELPER")
	if mode == "" {
		return
	}
	dir := os.Getenv("STEERLINE_TESTPROCESS_DIR")
	if mode == "parent" {
		for i := 0; i < 8; i++ {
			cmd := exec.Command(os.Args[0], "-test.run=^TestOwnedProcessHelper$")
			cmd.Env = append(os.Environ(), "STEERLINE_TESTPROCESS_HELPER=child", fmt.Sprintf("STEERLINE_TESTPROCESS_INDEX=%d", i))
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			// Release the parent-side process handle; the Job owns the descendant.
			cmd.Process.Release()
		}
		for {
			if _, err := os.Stat(filepath.Join(dir, "exit-parent")); err == nil {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	i := os.Getenv("STEERLINE_TESTPROCESS_INDEX")
	path, err := windows.UTF16PtrFromString(filepath.Join(dir, "locked-"+i))
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.CREATE_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	if err := os.WriteFile(filepath.Join(dir, "child-"+i+".pid"), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		t.Fatal(err)
	}
	for {
		time.Sleep(time.Hour)
	}
}
