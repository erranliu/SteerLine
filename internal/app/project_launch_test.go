package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	mv "github.com/erranliu/SteerLine/internal/manualverification"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

func TestProjectLaunchUsesReadyManagedCheckout(t *testing.T) {
	ctx := t.Context()
	m, _ := pmsFixture(t)
	cfg := m.config
	cfg.RunDir = t.TempDir()
	cfg.runtimeMaintenance = m
	require(t, cfg.ResolveIdentity(ctx))
	store, err := sqlite.OpenSQLiteStore(filepath.Join(cfg.RunDir, "test.db"))
	require(t, err)
	defer store.Close()
	s := newManualVerification(cfg, store)
	defer s.close()
	port := &runtimeProjectPort{config: cfg, manual: s, store: store, instance: "instance"}

	launch := mv.Launch{Program: "sh", Args: []string{"-c", "pwd; sleep 30"}}
	if runtime.GOOS == "windows" {
		launch = mv.Launch{Program: "powershell.exe", Args: []string{"-NoProfile", "-Command", "(Get-Location).Path; Start-Sleep -Seconds 30"}}
	}
	manifest := filepath.Join(cfg.Repository, filepath.FromSlash(mv.ProjectLaunchPath))
	raw, err := json.Marshal(map[string]any{"version": 1, "entries": []mv.ProjectLaunch{
		{ID: "interactive", Name: "Launch interactive", Launch: launch},
		{ID: "editor", Name: "Open editor", Launch: launch},
	}})
	require(t, err)
	require(t, os.WriteFile(manifest, raw, 0600))
	view, err := s.projectLaunchView(ctx, m)
	require(t, err)
	if view.Ready || len(view.Entries) != 0 || !strings.Contains(view.Diagnostic, mv.ProjectLaunchPath) {
		t.Fatalf("uncommitted manifest was accepted: %+v", view)
	}
	pmGit(t, cfg.Repository, "add", mv.ProjectLaunchPath)
	pmGit(t, cfg.Repository, "-c", "commit.gpgsign=false", "commit", "-qm", "manual launch")
	pmGit(t, cfg.Repository, "push", "origin", "main")
	observation, err := m.check(ctx)
	require(t, err)
	head := observation.Commit
	require(t, m.refresh(ctx))
	project := httpcontract.ProjectID(cfg.Identity())
	request := func(id, action, payload string) httpcontract.CommandEnvelope {
		return httpcontract.CommandEnvelope{ProjectID: project, InstanceID: port.instance, RequestID: strings.Repeat(id, 32), Action: "project_launch_" + action, TargetID: project, ExpectedVersion: 1, Payload: json.RawMessage(payload)}
	}
	start := func(id, head string) httpcontract.CommandEnvelope {
		return request(id, "start", `{"head":"`+head+`","entry_id":"interactive"}`)
	}
	dirty := filepath.Join(cfg.Repository, "untracked.txt")
	require(t, os.WriteFile(dirty, []byte("dirty"), 0600))
	result, err := port.Command(ctx, start("a", head))
	require(t, err)
	if result.Status != "rejected" || !strings.Contains(result.Diagnostic, "项目未就绪") {
		t.Fatal(result)
	}
	require(t, os.Remove(dirty))
	result, err = port.Command(ctx, start("b", strings.Repeat("a", 40)))
	require(t, err)
	if result.Status != "rejected" || !strings.Contains(result.Diagnostic, "提交已变化") {
		t.Fatal(result)
	}
	in := start("c", head)
	result, err = port.Command(ctx, in)
	require(t, err)
	if result.Status != "applied" {
		t.Fatal(result)
	}
	replayed, err := port.Command(ctx, in)
	require(t, err)
	if replayed.Status != "applied" {
		t.Fatal(replayed)
	}
	if _, err := m.updateLocal(ctx, strings.Repeat("d", 32)); err == nil || !strings.Contains(err.Error(), "正在用于项目启动") {
		t.Fatalf("sync was not gated: %v", err)
	}
	dir := filepath.Join(s.projectLaunchDir(), in.RequestID)
	deadline := time.Now().Add(10 * time.Second)
	for {
		logs, err := s.readLaunchLogs(dir)
		require(t, err)
		if strings.Contains(logs["stdout"], cfg.Repository) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("missing managed directory in launch output: %+v", logs)
		}
		time.Sleep(50 * time.Millisecond)
	}
	result, err = port.Command(ctx, request("e", "stop", `{"attempt_id":"`+in.RequestID+`"}`))
	require(t, err)
	if result.Status != "applied" {
		t.Fatal(result)
	}
	s.workers.Wait()
	view, err = s.projectLaunchView(ctx, m)
	require(t, err)
	if view.Run == nil || view.Run.Status != "stopped" || view.Run.Worktree != "" || view.Run.Head != head || len(view.Entries) != 2 {
		t.Fatalf("unexpected launch state: %+v", view)
	}
	if !m.checkoutMu.TryLock() {
		t.Fatal("stopped launch retained checkout lock")
	}
	m.checkoutMu.Unlock()
}
