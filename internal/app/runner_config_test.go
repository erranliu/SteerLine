package app

import (
	"context"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	"os"
	"path/filepath"
	"testing"
)

func TestRunnerTaskRepositoryKeepsConfiguredIdentityAcrossTransportRewrite(t *testing.T) {
	config, _ := nativeTaskFixture(t)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "url.http://127.0.0.1:1/fixture.git.insteadOf")
	t.Setenv("GIT_CONFIG_VALUE_0", "https://github.com/owner/repo.git")
	target, err := config.TaskRepositoryTarget(context.Background())
	require(t, err)
	if target != "https://github.com/owner/repo.git" {
		t.Fatal("transport rewrite replaced Task repository identity", target)
	}
}
func TestRunnerProjectPreflightLeavesPrivateRunnerFilesToRunner(t *testing.T) {
	config, _ := nativeTaskFixture(t)
	store, err := OpenNativeTaskProject(context.Background(), &config)
	require(t, err)
	require(t, store.Close())
	dir := runnerstatepath.Directory(config.RunDir)
	require(t, os.MkdirAll(dir, 0700))
	sentinel := filepath.Join(dir, "private-sentinel")
	require(t, os.WriteFile(sentinel, []byte("owned by Runner"), 0600))
	require(t, PreflightNativeStatePath(config))
	body, err := os.ReadFile(sentinel)
	require(t, err)
	if string(body) != "owned by Runner" {
		t.Fatal("SteerLine interpreted or modified Runner private data")
	}
	if _, err := os.Stat(filepath.Join(dir, "runner.db")); !os.IsNotExist(err) {
		t.Fatal("project preflight started Runner")
	}
}
