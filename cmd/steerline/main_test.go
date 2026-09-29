package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
)

func TestProjectsMenuCanOpenCurrentConsole(t *testing.T) {
	for _, action := range []string{"console", "1"} {
		got, err := pickAction(action)
		if err != nil || got != "console" {
			t.Fatalf("pickAction(%q) = %q, %v; want console", action, got, err)
		}
	}
}

func TestWaitServiceCommandResolvesIdentityAndKeepsPolling(t *testing.T) {
	repository := t.TempDir()
	if output, err := exec.Command("git", "init", repository).CombinedOutput(); err != nil {
		t.Fatalf("initialize test repository: %v\n%s", err, output)
	}
	config := steerline.Config{
		Repository: repository,
		Remote:     "origin",
		Branch:     "main",
		RunDir:     filepath.Join(t.TempDir(), "state"),
		Git:        steerline.Command{Executable: "git"},
	}
	resolvedConfig := config
	if err := resolvedConfig.ResolveIdentity(context.Background()); err != nil {
		t.Fatalf("resolve test repository identity: %v", err)
	}
	store, err := steerline.OpenSQLiteStore(filepath.Join(resolvedConfig.RunDir, "steerline.db"))
	if err != nil {
		t.Fatalf("create test command store: %v", err)
	}
	defer store.Close()

	commands := steerline.CommandService{Store: store, Project: resolvedConfig.Identity()}
	command := steerline.CommandRecord{
		CommandID:       strings.Repeat("a", 32),
		Kind:            "test_command",
		TargetID:        "test-target",
		ExpectedVersion: 1,
		IdempotencyKey:  "test-command-idempotency",
		RequestedBy:     "tester",
		Payload:         []byte(`{}`),
		Status:          "pending",
		CreatedAt:       time.Now().UTC(),
	}
	commandWritten := make(chan error, 1)
	go func() {
		time.Sleep(250 * time.Millisecond)
		queued, _, submitErr := commands.Submit(context.Background(), command)
		if submitErr != nil {
			commandWritten <- submitErr
			return
		}
		commandWritten <- commands.SetStatus(context.Background(), queued, "accepted", []byte(`{}`))
	}()

	got, err := waitServiceCommand(context.Background(), config, command.CommandID, 5*time.Second)
	if err != nil || got.Status != "accepted" {
		t.Fatalf("wait for delayed accepted command = %+v, %v; want accepted", got, err)
	}
	if err := <-commandWritten; err != nil {
		t.Fatalf("write delayed command receipt: %v", err)
	}

	_, err = waitServiceCommand(context.Background(), config, "not-visible-yet", 30*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "remains unconfirmed") {
		t.Fatalf("missing command should remain pending until timeout; got %v", err)
	}
}
