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

// RemoteTraceMain is a transparent test CLI adapter. It preserves every argv,
// stdout byte and native failure; only bounded lifecycle metadata is logged.
// No launcher override, credential or Dispatch capability is persisted.
func RemoteTraceMain(args []string) error {
	var config struct{ Executable, Output string }
	raw, err := os.ReadFile(filepath.Join(os.Getenv("STEERLINE_HOME"), "remote-trace.json"))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, &config); err != nil {
		return err
	}
	cmd := exec.Command(config.Executable, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	callErr := testprocess.Run(context.Background(), cmd)
	if len(args) > 1 && args[0] == "orchestration" && (args[1] == "worker-start" || args[1] == "check") {
		var envelope struct {
			OK     bool
			Result struct {
				State, Stage, FailedStage, RunID, TaskID, DispatchID, DeliveryID string
				Prompt                                                           struct{ Stages []string }
				Launch                                                           struct {
					Effective struct{ Agent, Model, Effort string }
				}
				Mutation struct{ RequestID string }
			}
			Error struct{ Code, Message string }
			Meta  struct{ RuntimeID string } `json:"_meta"`
		}
		parseErr := json.Unmarshal(stdout.Bytes(), &envelope)
		log, _ := json.Marshal(map[string]any{"args": args, "receipt": envelope, "parse_error": fmt.Sprint(parseErr), "call_error": fmt.Sprint(callErr)})
		if writeErr := os.WriteFile(filepath.Join(config.Output, fmt.Sprintf("remote-lifecycle-%d.json", time.Now().UnixNano())), log, 0600); writeErr != nil {
			return errors.Join(callErr, writeErr)
		}
	}
	_, outErr := os.Stdout.Write(stdout.Bytes())
	_, stderrErr := os.Stderr.Write(stderr.Bytes())
	return errors.Join(callErr, outErr, stderrErr)
}
