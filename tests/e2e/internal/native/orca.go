package native

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type OrcaEnvironment struct{ Executable, LogDir string }

func OrcaMain(args []string) error {
	raw, err := os.ReadFile(filepath.Join(os.Getenv("STEERLINE_HOME"), "orca-env.json"))
	if err != nil {
		return err
	}
	var c OrcaEnvironment
	if err = json.Unmarshal(raw, &c); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.Command(c.Executable, args...)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	callErr := testprocess.Run(ctx, cmd)
	if c.LogDir != "" {
		record, _ := json.Marshal(map[string]any{"args": args, "stdout": out.String(), "stderr": stderr.String(), "error": fmt.Sprint(callErr)})
		if err = os.WriteFile(filepath.Join(c.LogDir, fmt.Sprintf("native-%d.json", time.Now().UnixNano())), record, 0600); err != nil {
			return err
		}
	}
	_, oe := os.Stdout.Write(out.Bytes())
	_, se := os.Stderr.Write(stderr.Bytes())
	return errors.Join(callErr, oe, se)
}
