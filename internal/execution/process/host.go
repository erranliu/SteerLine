package process

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// Host implements only the small process/file port consumed by execution
// adapters. It has no access to the service or its persistence.
type Host struct{ Timeout time.Duration }

func New(timeout time.Duration) Host { return Host{Timeout: timeout} }

func (host Host) StartManaged(command *exec.Cmd) (executionbackend.StopHandle, error) {
	return StartManaged(command)
}

func (Host) InspectProcess(identity *executionbackend.ProcessIdentity) string {
	return InspectProcess(identity)
}

func (Host) NewProcessGeneration() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}

func (Host) WriteExecutionFile(path string, value []byte) error { return writeAtomic(path, value) }

func (host Host) OutputCommand(ctx context.Context, command executionbackend.LaunchCommand) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	timeout := command.Timeout
	if timeout <= 0 {
		timeout = host.Timeout
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.Command(command.Executable, command.Args...)
	cmd.Dir = command.WorkingDirectory
	output, diagnostics := boundedBuffer{max: 16 << 20}, boundedBuffer{max: 1 << 20}
	cmd.Stdout, cmd.Stderr = &output, &diagnostics
	owned, err := host.StartManaged(cmd)
	if err != nil {
		return nil, err
	}
	defer owned.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		owned.Kill()
		<-done
		err = ctx.Err()
	}
	if err != nil {
		return output.Bytes(), fmt.Errorf("%s failed: %w (%s)", command.Executable, err, diagnostics.String())
	}
	return output.Bytes(), nil
}

type boundedBuffer struct {
	bytes.Buffer
	max int
}

func (b *boundedBuffer) Write(data []byte) (int, error) {
	if b.Len()+len(data) > b.max {
		return 0, errors.New("command output exceeds its read limit")
	}
	return b.Buffer.Write(data)
}

func writeAtomic(path string, value []byte) error {
	if path == "" {
		return errors.New("execution file path is empty")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".steerline-execution-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(value)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return replaceFile(tmpPath, path)
}
