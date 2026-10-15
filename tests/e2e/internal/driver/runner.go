package driver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerclient"
)

// RunnerSelection uses the explicitly supplied standalone executable. It never
// builds another repository or reads Runner-private state.
func RunnerSelection(executable string) (runnerclient.Selection, error) {
	if executable == "" {
		executable = os.Getenv("STEERRUNNER_EXE")
	}
	if executable == "" {
		var err error
		executable, err = exec.LookPath("steerrunner.exe")
		if err != nil {
			return runnerclient.Selection{}, errors.New("select a standalone Runner with STEERRUNNER_EXE")
		}
	}
	if !filepath.IsAbs(executable) {
		return runnerclient.Selection{}, errors.New("Runner executable must be absolute")
	}
	body, err := os.ReadFile(executable)
	if err != nil {
		return runnerclient.Selection{}, err
	}
	sum := sha256.Sum256(body)
	selection := runnerclient.Selection{Executable: executable, SHA256: hex.EncodeToString(sum[:])}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cli := runnerclient.Client{Executable: executable, SHA256: selection.SHA256}
	version, err := cli.Version(ctx)
	if err != nil {
		return selection, err
	}
	selection.Version = version.Version
	return selection, nil
}
