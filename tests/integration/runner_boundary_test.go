package integration

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The actual cross-process behavior is exercised by the independently launched
// TestDebugReleaseRuntime driver in a newly generated SteerlineDebug package.
func TestRunnerExecutionBoundary(t *testing.T) {
	for _, owner := range []string{"adapters", "reliable", "workflow", "conversation"} {
		if _, err := os.Stat("../../internal/execution/" + owner); !os.IsNotExist(err) {
			t.Fatal("retired execution owner returned", owner, err)
		}
	}
	for _, target := range []string{"windows", "linux"} {
		t.Run(target, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, "go", "list", "-deps", "../../cmd/steerline")
			for _, item := range os.Environ() {
				key, _, _ := strings.Cut(item, "=")
				if !strings.EqualFold(key, "GOOS") && !strings.EqualFold(key, "GOARCH") {
					cmd.Env = append(cmd.Env, item)
				}
			}
			cmd.Env = append(cmd.Env, "GOOS="+target, "GOARCH=amd64")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatal(err, string(output))
			}
			checked := 0
			for _, line := range strings.Fields(string(output)) {
				checked++
				for _, private := range []string{"github.com/erranliu/SteerRunner/", "github.com/erranliu/SteerLine/internal/execution/adapters", "github.com/erranliu/SteerLine/internal/execution/reliable", "github.com/erranliu/SteerLine/internal/execution/workflow"} {
					if strings.HasPrefix(line, private) {
						t.Fatal("product retained an execution implementation", line)
					}
				}
			}
			if checked == 0 || !strings.Contains(string(output), "/internal/runnerclient") {
				t.Fatal("no public Runner CLI consumer in product dependency closure")
			}
		})
	}
}
