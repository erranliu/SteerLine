package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/prompts"
	progresstools "github.com/erranliu/SteerLine/tools"
)

func TestProjectRegistrationPromptUsesDirectoryHomeWithoutProjects(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	home := filepath.Join(t.TempDir(), "SteerLine 安装")
	d := NewApplicationDirectory(home)
	defer d.cancel()
	result, err := d.ProjectRegistrationPrompt()
	if err != nil || !strings.Contains(result.Prompt, home) || strings.Contains(result.Prompt, HomeDir()) {
		t.Fatalf("prompt did not use the application directory home: %q, %v", result.Prompt, err)
	}
	if !strings.Contains(result.InitializationPrompt, "https://github.com/erranliu/SteerLine/blob/HEAD/SteerlineProjectGuideDoc/README.md") ||
		strings.Contains(result.InitializationPrompt, home) {
		t.Fatalf("unexpected project initialization prompt: %q", result.InitializationPrompt)
	}
	if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("prompt wrote to the installation directory: %v", err)
	}
}

func TestProjectRegistrationUsesPublishedCommitAndStagesConfiguration(t *testing.T) {
	root, home, repo := t.TempDir(), t.TempDir(), t.TempDir()
	d := NewApplicationDirectory(home)
	defer d.cancel()
	pmGit(t, repo, "init", "-b", "main")
	pmGit(t, repo, "config", "user.email", "test@example.invalid")
	pmGit(t, repo, "config", "user.name", "Project registration test")
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"README.md", "AGENTS.md", "SteerlineFlow/paths.md", "SteerlineFlow/testing.md",
		"SteerlineProjectGuideDoc/README.md", "SteerlineProjectGuideDoc/project-initialization.md",
		"SteerlineProjectGuideDoc/project-standards.md", "SteerlineProjectGuideDoc/environment-adaptation.md",
		"SteerlineProjectGuideDoc/spec-plan-guidelines.md", "SteerlineProjectGuideDoc/testing-architecture.md"} {
		write(name, "# Project\n")
	}
	rules, err := prompts.TestingRules()
	if err != nil {
		t.Fatal(err)
	}
	write("SteerlineFlow/testing/runtime-progress.md", string(rules))
	write("tools/testing/RuntimeProgress.ps1", progresstools.PowerShellWriter)
	write("SteerlineFlow/environment-project.json", `{"version":1,"requirements":["SteerlineFlow/testing.md"],"testing_catalogue":"SteerlineFlow/testing/test-list.json"}`)
	write("SteerlineFlow/testing/test-list.json", `{"tests":[]}`)
	write("SteerlineFlow/environment.md", "```steerline-environment\n{\"version\":6,\"script\":\"SteerlineFlow/scripts/verify-environment.sh\",\"interpreter\":\"bash\",\"dependencies\":[],\"requirements\":{}}\n```\n")
	pmGit(t, repo, "add", ".")
	pmGit(t, repo, "commit", "-m", "missing environment entry")
	bare := filepath.Join(root, "owner", "project.git")
	if err := os.MkdirAll(filepath.Dir(bare), 0700); err != nil {
		t.Fatal(err)
	}
	pmGit(t, root, "clone", "--bare", repo, bare)
	pmGit(t, bare, "update-server-info")
	remote := httptest.NewServer(http.FileServer(http.Dir(root)))
	defer remote.Close()
	pmGit(t, repo, "remote", "add", "origin", remote.URL+"/owner/project.git")
	pmGit(t, repo, "remote", "add", "fixture-publish", bare)
	input := contract.ProjectRegistration{Name: "project", Repository: repo, Remote: "origin",
		Branch: "main", Provider: "gitea", BaseURL: remote.URL, PRRepository: "owner/project"}
	ctx := context.Background()
	write("SteerlineFlow/scripts/verify-environment.sh", "#!/bin/bash\nexit 1\n")
	if _, err := d.RegisterProject(ctx, input); err == nil || !strings.Contains(err.Error(), "verify-environment.sh") {
		t.Fatalf("an uncommitted entry must not satisfy the target branch: %v", err)
	}
	publish := func() string {
		t.Helper()
		pmGit(t, repo, "add", ".")
		pmGit(t, repo, "commit", "-m", "publish initialization")
		pmGit(t, repo, "push", "fixture-publish", "main")
		pmGit(t, bare, "update-server-info")
		return pmGit(t, repo, "rev-parse", "HEAD")
	}
	commit := publish()
	check, err := d.RegisterProject(ctx, input)
	if err != nil || check.Commit != commit || check.Saved || check.Confirmation == "" {
		t.Fatalf("preview = %+v, %v", check, err)
	}
	for _, path := range []string{filepath.Join(home, "etc"), filepath.Join(home, "state")} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("preview wrote %s: %v", path, err)
		}
	}
	write("README.md", "# Published change\n")
	commit = publish()
	input.Confirmation = check.Confirmation
	var conflict *httpclient.ClientError
	if _, err = d.RegisterProject(ctx, input); !errors.As(err, &conflict) || conflict.Code != "registration_changed" {
		t.Fatalf("stale confirmation = %v", err)
	}
	input.Confirmation = ""
	check, err = d.RegisterProject(ctx, input)
	if err != nil || check.Commit != commit {
		t.Fatalf("new preview = %+v, %v", check, err)
	}
	input.Confirmation = check.Confirmation
	for range 2 {
		result, err := d.RegisterProject(ctx, input)
		if err != nil || !result.Saved || result.ProjectID != check.ProjectID {
			t.Fatalf("confirm or retry = %+v, %v", result, err)
		}
	}
	if len(d.entries) != 0 {
		t.Fatal("current running directory changed before restart")
	}
	next := NewApplicationDirectory(home)
	defer next.cancel()
	if len(next.entries) != 1 || next.entries[0].view.ConfigID != "project.json" {
		t.Fatalf("startup did not load the saved config: %+v", next.entries)
	}
	config, err := DecodeProjectConfigInput(next.entries[0].data)
	if err != nil || canonicalPath(config.Repository) != canonicalPath(repo) || config.Remote != "origin" || config.Branch != "main" || config.PR.Repository != "owner/project" || config.TaskAuthority != "native" {
		t.Fatalf("saved configuration = %+v, %v", config, err)
	}
	if _, err := os.Stat(filepath.Join(home, "state")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("registration initialized project state: %v", err)
	}
	input.Name, input.Confirmation = "duplicate", ""
	if _, err := d.RegisterProject(ctx, input); !errors.As(err, &conflict) || conflict.Code != "project_already_registered" {
		t.Fatalf("same repository under another name = %v", err)
	}
}
