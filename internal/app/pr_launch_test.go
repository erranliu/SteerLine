package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	mv "github.com/erranliu/SteerLine/internal/manualverification"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

func TestPRLaunchUsesSelectedHeadAndOriginalWorktreeDirectory(t *testing.T) {
	ctx := t.Context()
	m, _ := pmsFixture(t)
	cfg := m.config
	cfg.RunDir = t.TempDir()
	cfg.runtimeMaintenance = m
	require(t, cfg.ResolveIdentity(ctx))
	store, err := sqlite.OpenSQLiteStore(filepath.Join(cfg.RunDir, "test.db"))
	require(t, err)
	defer store.Close()

	launch := mv.Launch{Program: "sh", Args: []string{"-c", "cat marker.txt; echo keep > dirty.txt"}}
	if runtime.GOOS == "windows" {
		launch = mv.Launch{Program: "powershell.exe", Args: []string{"-NoProfile", "-Command", "Get-Content marker.txt; Set-Content dirty.txt keep"}}
	}
	body, err := json.Marshal(map[string]any{"version": 1, "entries": []mv.ProjectLaunch{{ID: "preview", Name: "Preview", Launch: launch}}})
	require(t, err)
	repo := cfg.Repository
	pmGit(t, repo, "checkout", "-b", "feature")
	require(t, os.WriteFile(filepath.Join(repo, mv.ProjectLaunchPath), body, 0600))
	require(t, os.WriteFile(filepath.Join(repo, "marker.txt"), []byte("PR-specific marker"), 0600))
	pmGit(t, repo, "add", mv.ProjectLaunchPath, "marker.txt")
	pmGit(t, repo, "-c", "commit.gpgsign=false", "commit", "-qm", "PR contents")
	head := pmGit(t, repo, "rev-parse", "HEAD")
	pmGit(t, repo, "checkout", "main")

	root := t.TempDir()
	remote := filepath.Join(root, "owner", "project.git")
	require(t, os.MkdirAll(filepath.Dir(remote), 0700))
	pmGit(t, root, "init", "--bare", remote)
	pmGit(t, repo, "push", remote, "feature:refs/pull/7/head")
	pmGit(t, remote, "update-server-info")
	fixturePull := func(r *http.Request, number int) roadmapPR {
		fact := roadmapPR{URL: fmt.Sprintf("http://%s/owner/project/pulls/%d", r.Host, number), Title: "Preview", State: "open", Number: number}
		fact.Head.Ref, fact.Head.SHA = "feature", head
		return fact
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/owner/project/pulls":
			if r.URL.RawQuery != "state=open&sort=recentupdate&limit=10" {
				http.Error(w, "unexpected query", 400)
				return
			}
			var facts []roadmapPR
			for number := 20; number > 8; number-- {
				facts = append(facts, fixturePull(r, number))
			}
			_ = json.NewEncoder(w).Encode(facts)
		case "/api/v1/repos/owner/project/pulls/7":
			_ = json.NewEncoder(w).Encode(fixturePull(r, 7))
		default:
			http.FileServer(http.Dir(root)).ServeHTTP(w, r)
		}
	}))
	defer provider.Close()
	pmGit(t, repo, "remote", "set-url", "origin", provider.URL+"/owner/project.git")
	cfg.PR = executionmodel.PRProviderConfig{Kind: "gitea", BaseURL: provider.URL, Repository: "owner/project"}
	s := newManualVerification(cfg, store)
	defer s.close()
	port := &runtimeProjectPort{manual: s, config: cfg, instance: "instance", store: store}

	view, err := s.projectLaunchView(ctx, m)
	require(t, err)
	if view.Ready {
		t.Fatal("managed checkout should be ineligible in fixture")
	}
	result, err := port.Read(ctx, "/manual-verification/pulls", nil)
	require(t, err)
	rows := result.([]launchPR)
	if len(rows) != 10 || rows[0].Number != 20 || rows[9].Number != 11 {
		t.Fatalf("recent PRs: %+v", rows)
	}
	detail, err := port.Read(ctx, "/manual-verification/pulls/7", nil)
	require(t, err)
	detailJSON, err := json.Marshal(detail)
	require(t, err)
	if !strings.Contains(string(detailJSON), `"name":"Preview"`) || strings.Contains(string(detailJSON), `"diagnostic"`) {
		t.Fatalf("entry must come from the selected Head: %s", detailJSON)
	}
	if _, err := cfg.setWorktreeDirectory(repo); err == nil {
		t.Fatal("managed checkout accepted as worktree base")
	}
	settings, err := cfg.setWorktreeDirectory(t.TempDir())
	require(t, err)
	project := httpcontract.ProjectID(cfg.Identity())
	request := func(id, action, payload string) httpcontract.CommandEnvelope {
		return httpcontract.CommandEnvelope{ProjectID: project, InstanceID: port.instance, RequestID: strings.Repeat(id, 32), Action: "project_launch_pr_" + action, TargetID: project, ExpectedVersion: 1, Payload: json.RawMessage(payload)}
	}
	start := func(id, sha string) httpcontract.CommandEnvelope {
		return request(id, "start", fmt.Sprintf(`{"head":%q,"pr_number":7,"entry_id":"preview"}`, sha))
	}
	receipt, err := port.Command(ctx, start("a", strings.Repeat("b", 40)))
	require(t, err)
	if receipt.Status != "rejected" || !strings.Contains(receipt.Diagnostic, "Head 已变化") {
		t.Fatal(receipt)
	}
	in := start("b", head)
	receipt, err = port.Command(ctx, in)
	require(t, err)
	if receipt.Status != "applied" {
		t.Fatal(receipt)
	}
	waitForLaunch := func() {
		t.Helper()
		done := make(chan struct{})
		go func() { s.workers.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(45 * time.Second):
			t.Fatal("launch did not complete")
		}
	}
	waitForLaunch()
	run := *s.prRun
	if run.Head != head || run.Status != "exited" || run.Removed || run.CleanupDiagnostic == "" ||
		run.Worktree != filepath.Join(settings.Effective, "manual-verification", run.ID, "worktree") {
		t.Fatalf("expected original dirty worktree to be retained: %+v", run)
	}
	logs, err := s.readLaunchLogs(filepath.Join(s.prLaunchDir(), run.ID))
	require(t, err)
	if !strings.Contains(logs["stdout"], "PR-specific marker") {
		t.Fatal(logs)
	}
	nextSettings, err := cfg.setWorktreeDirectory(t.TempDir())
	require(t, err)
	next := start("c", head)
	receipt, err = port.Command(ctx, next)
	require(t, err)
	if receipt.Status != "applied" {
		t.Fatal("retained worktree blocked another PR launch", receipt)
	}
	waitForLaunch()
	if nextRun := s.prRun; nextRun.ID != next.RequestID || !strings.Contains(nextRun.Worktree, nextSettings.Effective) {
		t.Fatalf("new launch did not use the updated directory: %+v", nextRun)
	}
	require(t, os.Remove(filepath.Join(run.Worktree, "dirty.txt")))
	receipt, err = port.Command(ctx, request("d", "cleanup", fmt.Sprintf(`{"attempt_id":%q}`, run.ID)))
	require(t, err)
	if receipt.Status != "applied" || s.prRunByID(run.ID) != nil {
		t.Fatal(receipt, s.prRun)
	}
	if _, err := os.Stat(run.Worktree); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("original worktree still exists: %v", err)
	}
	nextRun := *s.prRun
	require(t, os.Remove(filepath.Join(nextRun.Worktree, "dirty.txt")))
	require(t, s.finishPRLaunch(ctx, nextRun.ID, false))
	if !s.prRun.Removed {
		t.Fatal("latest worktree still exists", s.prRun.CleanupDiagnostic)
	}
}
