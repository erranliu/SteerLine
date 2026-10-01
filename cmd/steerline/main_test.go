package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
)

func TestCLIExecutionProfileIsBoundIntoStartTaskRequest(t *testing.T) {
	repository := t.TempDir()
	if output, err := exec.Command("git", "init", "-b", "main", repository).CombinedOutput(); err != nil {
		t.Fatalf("initialize repository: %v: %s", err, output)
	}
	worker := executionapi.PublicationPolicy{Publisher: "worker"}
	config := steerline.Config{
		Repository: repository, Git: steerline.Command{Executable: "git"},
		ExecutionPolicy:   executionapi.ExecutionPolicy{Filesystem: executionapi.FilesystemPolicy{ReadScope: "host", WriteScope: "workspace", GitMetadataWrite: true}, Network: "allow", Escalation: "deny"},
		PublicationPolicy: executionapi.PublicationPolicy{Publisher: steerline.PublisherExecutionProfile},
		PR:                publication.Config{Kind: "command", BaseURL: "https://github.com", Repository: "owner/repo", Query: steerline.Command{Executable: os.Args[0]}},
		ExecutionProfiles: []steerline.ExecutionProfile{{ID: "orca", Name: "Orca", Host: "orca", Agent: "codex", ConfiguredPublicationPolicy: &worker, Codex: steerline.Command{Executable: os.Args[0], WindowsSandbox: "unelevated"}}},
	}
	if _, err := startTaskControlPayload(context.Background(), &config, "101", ""); err == nil {
		t.Fatal("manual start silently selected a profile")
	}
	payload, err := startTaskControlPayload(context.Background(), &config, "101", "orca")
	if err != nil {
		t.Fatal(err)
	}
	_, selected, err := config.ResolveExecutionProfile("orca", payload["execution_profile_digest"])
	if err != nil || payload["task_id"] != "101" || payload["execution_profile_id"] != selected.ID || selected.PublicationPolicy.Publisher != "worker" {
		t.Fatalf("CLI request lost its selected route: %+v / %v", payload, err)
	}
}

func TestCLIExecutionProfileFlagRejectsUnrelatedActions(t *testing.T) {
	for _, args := range [][]string{
		{"check", "-execution-profile", "orca"},
		{"control", "-action", "resume_dispatch", "-execution-profile", "orca"},
		{"control", "-action", "start_task", "-request-id", "receipt", "-execution-profile", "orca"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			old := os.Args
			os.Args = append([]string{"steerline"}, args...)
			defer func() { os.Args = old }()
			if err := run(); err == nil || !strings.Contains(err.Error(), "only valid with control -action start_task") {
				t.Fatalf("execution selection accepted for another action: %v", err)
			}
		})
	}
}

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
