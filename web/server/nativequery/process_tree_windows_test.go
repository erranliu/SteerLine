//go:build windows

package nativequery

import (
	"bufio"
	"context"
	"fmt"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestNativeQueryCancellationSettlesOwnedDescendant(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := Start(ctx, t.TempDir(), queryExecutable(t), "--nativequery-fixture", "tree")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	pids := make(chan [2]int, 1)
	go func() {
		r := bufio.NewReader(p.Output)
		var parent, child int
		if _, err := fmt.Fscanf(r, "ready %d\n", &parent); err != nil {
			pids <- [2]int{}
			return
		}
		if _, err := p.Input.Write([]byte{1}); err != nil {
			pids <- [2]int{}
			return
		}
		if _, err := fmt.Fscanf(r, "child %d\n", &child); err != nil {
			pids <- [2]int{}
			return
		}
		pids <- [2]int{parent, child}
	}()
	var ids [2]int
	select {
	case ids = <-pids:
	case <-time.After(5 * time.Second):
		t.Fatal("owned process tree never became ready")
	}
	if ids[0] != p.cmd.Process.Pid || ids[1] <= 0 {
		t.Fatal("invalid process identity", ids)
	}
	var handles []windows.Handle
	for _, pid := range ids {
		handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
		if err != nil {
			t.Fatal(err)
		}
		defer windows.CloseHandle(handle)
		handles = append(handles, handle)
		if status, err := windows.WaitForSingleObject(handle, 0); err != nil || status != uint32(windows.WAIT_TIMEOUT) {
			t.Fatal("process was not alive before cancellation", pid, status, err)
		}
	}
	cancel()
	for i, handle := range handles {
		if status, err := windows.WaitForSingleObject(handle, 5000); err != nil || status != windows.WAIT_OBJECT_0 {
			t.Fatal("owned process survived cancellation", ids[i], status, err)
		}
	}
	done := make(chan struct{})
	go func() { _ = p.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("original cancelled process was not reaped within the boundary")
	}
	if ctx.Err() != context.Canceled || p.cmd.ProcessState == nil || p.cmd.ProcessState.Pid() != ids[0] {
		t.Fatal("original cancelled PID was not reaped", ids[0])
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
}
