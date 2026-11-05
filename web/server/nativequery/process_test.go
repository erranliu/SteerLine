package nativequery

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 2 && os.Args[1] == "--nativequery-fixture" {
		switch os.Args[2] {
		case "cwd":
			dir, err := os.Getwd()
			if err != nil {
				os.Exit(2)
			}
			fmt.Print(dir)
		case "large":
			_, _ = os.Stdout.Write(bytes.Repeat([]byte("x"), Limit+1))
		case "fail":
			os.Exit(17)
		case "tree":
			fmt.Fprintf(os.Stdout, "ready %d\n", os.Getpid())
			// The caller sends this only after Start has returned and attached
			// ownership, so descendants cannot race ahead of job assignment.
			if _, err := io.ReadFull(os.Stdin, make([]byte, 1)); err != nil {
				os.Exit(3)
			}
			child := exec.Command(os.Args[0], "--nativequery-fixture", "child")
			child.Stdout, child.Stderr = os.Stdout, os.Stderr
			if err := child.Start(); err != nil {
				os.Exit(4)
			}
			_ = child.Wait()
		case "child":
			fmt.Fprintf(os.Stdout, "child %d\n", os.Getpid())
			time.Sleep(time.Hour)
		case "wait":
			fmt.Fprintln(os.Stdout, "ready")
			time.Sleep(time.Hour)
		default:
			os.Exit(5)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func queryExecutable(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

func TestNativeQueryRunUsesHomeAndEnforcesOutputBoundary(t *testing.T) {
	home, exe := t.TempDir(), queryExecutable(t)
	for _, role := range []string{"cwd", "large", "fail"} {
		t.Run(role, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			body, err := Run(ctx, home, exe, []string{"--nativequery-fixture", role})
			if role == "cwd" {
				if err != nil || string(body) != home {
					t.Fatalf("cwd/output changed: %q %v", body, err)
				}
			} else if err == nil {
				t.Fatal("metadata failure was accepted", role)
			} else if role == "large" && !strings.Contains(err.Error(), "exceeds 1 MiB") {
				t.Fatal("oversize did not reach output boundary", err)
			}
		})
	}
}

func TestNativeQueryExecutableIgnoresRelativeSearchDirectories(t *testing.T) {
	root, allowed := t.TempDir(), t.TempDir()
	t.Chdir(root)
	name := "metadata-fixture"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	relative := filepath.Join(root, "relative")
	if err := os.Mkdir(relative, 0700); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{root, relative, allowed} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("lookup fixture"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", "."+string(os.PathListSeparator)+allowed)
	resolved, err := Executable(name)
	if err != nil || resolved != filepath.Join(allowed, name) {
		t.Fatal("cwd shadowed absolute PATH", resolved, err)
	}
	t.Setenv("PATH", "relative"+string(os.PathListSeparator)+allowed)
	if resolved, err = Executable(name); err != nil || resolved != filepath.Join(allowed, name) {
		t.Fatal("relative PATH directory shadowed absolute PATH", resolved, err)
	}
	if resolved, err = Executable(filepath.Join(allowed, name)); err != nil || resolved != filepath.Join(allowed, name) {
		t.Fatal(resolved, err)
	}
	for _, program := range []string{"", "./" + name, "sub/" + name, "../" + name} {
		if _, err := Executable(program); err == nil {
			t.Fatal("relative program accepted", program)
		}
	}
	t.Setenv("PATH", "."+string(os.PathListSeparator)+"relative")
	if _, err := Executable(name); err == nil {
		t.Fatal("relative-only PATH resolved a program from cwd")
	}
	t.Setenv("PATH", "relative")
	if _, err := Executable(name); err == nil {
		t.Fatal("relative-only PATH resolved a program from a relative directory")
	}
}

func TestNativeQueryStartRejectsRelativePaths(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	relativeHome := filepath.Join(root, "relative")
	if err := os.Mkdir(relativeHome, 0700); err != nil {
		t.Fatal(err)
	}
	name := "nativequery-fixture"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	fixture := filepath.Join(root, name)
	binary, err := os.ReadFile(queryExecutable(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture, binary, 0700); err != nil {
		t.Fatal(err)
	}
	// Prove both homes and the copied executable really work. Otherwise an OS
	// lookup failure could masquerade as rejection by Start's path contract.
	for _, home := range []string{root, relativeHome} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		p, err := Start(ctx, home, fixture, "--nativequery-fixture", "cwd")
		if err != nil {
			t.Fatal("absolute fixture control did not start", err)
		}
		t.Cleanup(func() { _ = p.Close() })
		body, readErr := io.ReadAll(p.Output)
		waitErr := p.Wait()
		if readErr != nil || waitErr != nil || string(body) != home {
			t.Fatal("absolute fixture control failed", string(body), readErr, waitErr)
		}
		if err := p.Close(); err != nil {
			t.Fatal(err)
		}
	}
	for _, paths := range [][2]string{{"relative", queryExecutable(t)}, {t.TempDir(), "relative.exe"}} {
		p, err := Start(context.Background(), paths[0], paths[1], "--nativequery-fixture", "cwd")
		if err == nil {
			_ = p.Close()
			_ = p.Wait()
			t.Fatal("relative metadata path accepted", paths)
		}
	}
	for _, paths := range [][2]string{{"relative", fixture}, {root, "./" + name}} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		p, err := Start(ctx, paths[0], paths[1], "--nativequery-fixture", "cwd")
		if err == nil {
			_ = p.Close()
			_ = p.Wait()
			t.Fatal("real relative metadata fixture accepted", paths)
		}
	}
}

func TestNativeQueryCancellationSettlesOriginalProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := Start(ctx, t.TempDir(), queryExecutable(t), "--nativequery-fixture", "wait")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	ready := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(p.Output).ReadString('\n'); ready <- line }()
	select {
	case line := <-ready:
		if line != "ready\n" {
			t.Fatal("process not ready", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("metadata process did not start")
	}
	cancel()
	done := make(chan error, 1)
	go func() { done <- p.Wait() }()
	select {
	case <-done:
		// Wait's numeric status/error can race with CommandContext's own
		// cancellation. The contract here is settlement of this exact PID.
		if ctx.Err() != context.Canceled || p.cmd.ProcessState == nil || p.cmd.ProcessState.Pid() != p.cmd.Process.Pid {
			t.Fatal("cancelled original process did not settle")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled metadata process did not settle")
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
}
