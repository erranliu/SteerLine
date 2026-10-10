package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestProcessHostCommandHelper(t *testing.T) {
	if !slices.Contains(os.Args, "-test.run=^TestProcessHostCommandHelper$") {
		t.Skip("subprocess helper")
	}
	time.Sleep(200 * time.Millisecond)
	fmt.Println("command completed")
	fmt.Println("routing=" + os.Getenv("ORCA_ENVIRONMENT"))
	fmt.Println("kept=" + os.Getenv("STEERLINE_VIEWER_TEST_KEEP"))
}

func TestProcessHostBlocksAmbientRoutingOnly(t *testing.T) {
	t.Setenv("ORCA_ENVIRONMENT", "foreign-host")
	t.Setenv("STEERLINE_VIEWER_TEST_KEEP", "original")
	out, err := New(5*time.Second).OutputCommand(context.Background(), executionbackend.LaunchCommand{Executable: os.Args[0], Args: []string{"-test.run=^TestProcessHostCommandHelper$"}, BlockedEnvironment: []string{"orca_environment"}})
	if err != nil || strings.Contains(string(out), "foreign-host") || !strings.Contains(string(out), "kept=original") || os.Getenv("ORCA_ENVIRONMENT") != "foreign-host" {
		t.Fatalf("routing filtering changed parent/unrelated environment: %s / %v", out, err)
	}
}

func TestProcessHostOperationBudget(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := executionbackend.LaunchCommand{Executable: executable, Args: []string{"-test.run=^TestProcessHostCommandHelper$"}}
	for _, scenario := range []string{"explicit_budget", "default_budget", "parent_cancel"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			cmd := command
			if scenario != "default_budget" {
				cmd.Timeout = 5 * time.Second
			}
			if scenario == "parent_cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
			}
			out, err := New(50*time.Millisecond).OutputCommand(ctx, cmd)
			if scenario == "explicit_budget" {
				if err != nil || !strings.Contains(string(out), "command completed") {
					t.Fatalf("ordinary host timeout truncated operation budget: %s / %v", out, err)
				}
			} else if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("command ignored default or parent deadline: %s / %v", out, err)
			}
		})
	}
}
