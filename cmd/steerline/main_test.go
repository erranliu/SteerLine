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

func TestWatchFlagsRouteTTYToStreamingMode(t *testing.T) {
	cases := []struct {
		name                                string
		json, raw, noTUI, follow, fromStart bool
		want                                bool
	}{
		{name: "default TTY viewer", follow: true, fromStart: true, want: true},
		{name: "follow false streams", follow: false, fromStart: true},
		{name: "from start false streams", follow: true, fromStart: false},
		{name: "JSON streams", json: true, follow: true, fromStart: true},
		{name: "raw streams", raw: true, follow: true, fromStart: true},
		{name: "no TUI streams", noTUI: true, follow: true, fromStart: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := useWatchTUI(true, tc.json, tc.raw, tc.noTUI, tc.follow, tc.fromStart); got != tc.want {
				t.Fatalf("useWatchTUI=%v want %v", got, tc.want)
			}
		})
	}
}

func TestWaitServiceCommandKeepsPollingWhenCommandIsNotVisibleYet(t *testing.T) {
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
	if err := config.ResolveIdentity(context.Background()); err != nil {
		t.Fatalf("resolve test repository identity: %v", err)
	}
	store, err := steerline.OpenSQLiteStore(filepath.Join(config.RunDir, "steerline.db"))
	if err != nil {
		t.Fatalf("create test command store: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close test command store: %v", err)
	}

	_, err = waitServiceCommand(context.Background(), config, "not-visible-yet", 30*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "remains unconfirmed") {
		t.Fatalf("missing command should remain pending until timeout; got %v", err)
	}
}
