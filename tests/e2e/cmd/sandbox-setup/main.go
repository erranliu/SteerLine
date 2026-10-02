// sandbox-setup is explicit machine preparation, separate from test execution.
// It calls native Codex provisioning only after an unsuccessful read-only check.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/testprocess"
	"github.com/erranliu/SteerLine/tests/e2e/internal/native"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Sandbox setup:", err)
		os.Exit(1)
	}
}

func run() error {
	home := flag.String("codex-home", "", "Codex home reused as live E2E credential source")
	command := flag.String("codex", "codex", "installed Codex executable or npm command shim")
	checkOnly := flag.Bool("check-only", false, "check existing accounts without requesting elevation")
	timeout := flag.Duration("timeout", 5*time.Minute, "maximum time for native provisioning, including UAC")
	flag.Parse()
	if runtime.GOOS != "windows" {
		return errors.New("requires native Windows")
	}
	if flag.NArg() != 0 || *timeout < 30*time.Second || *timeout > 15*time.Minute {
		return errors.New("unexpected arguments or timeout outside 30s..15m")
	}
	if *home == "" {
		*home = os.Getenv("CODEX_HOME")
		if *home == "" {
			userHome, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			*home = filepath.Join(userHome, ".codex")
		}
	}
	absHome, err := filepath.Abs(*home)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "Codex home:", absHome)
	return ensureSandbox(absHome, *checkOnly, native.CheckSandboxReady, func() error {
		executable, err := native.CodexExecutable(*command)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		var version bytes.Buffer
		versionCmd := exec.Command(executable, "--version")
		versionCmd.Stdout, versionCmd.Stderr = &version, &version
		versionCtx, versionCancel := context.WithTimeout(ctx, 10*time.Second)
		err = testprocess.Run(versionCtx, versionCmd)
		versionCancel()
		if err != nil {
			return fmt.Errorf("read Codex version: %w", err)
		}
		if strings.TrimSpace(version.String()) != "codex-cli 0.159.2" {
			return errors.New("live E2E setup requires pinned codex-cli 0.159.2")
		}
		if err := os.MkdirAll(absHome, 0700); err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, "Requesting native Codex setup. Approve the Windows administrator prompt to continue.")
		cmd, err := provisioningCommand(executable, absHome, *timeout)
		if err != nil {
			return err
		}
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := testprocess.Run(ctx, cmd); err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("native setup timed out: %w", ctx.Err())
			}
			return fmt.Errorf("native setup failed or authorization was cancelled: %w", err)
		}
		return nil
	}, os.Stdout)
}

func ensureSandbox(home string, checkOnly bool, ready func(string) error, provision func() error, output io.Writer) error {
	if err := ready(home); err == nil {
		fmt.Fprintln(output, "Sandbox ready. Reusing existing setup; no administrator prompt needed.")
		return nil
	} else {
		if checkOnly {
			return fmt.Errorf("read-only check failed: %w", err)
		}
		fmt.Fprintln(output, "Existing sandbox needs initialization/repair:", err)
	}
	if err := provision(); err != nil {
		return err
	}
	if err := ready(home); err != nil {
		return fmt.Errorf("native setup completed but account verification failed: %w", err)
	}
	fmt.Fprintln(output, "Sandbox initialized and both accounts verified. Live tests can reuse this home.")
	return nil
}
