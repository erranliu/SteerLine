package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
)

func TestDeploymentPreparation(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	runner := filepath.Join(t.TempDir(), "staged-runner")
	require(t, os.WriteFile(runner, []byte("fixed staged bytes"), 0600))
	cycles := 0
	cycle := func(context.Context) error { cycles++; t.Fatal("unexpected recovery"); return nil }
	for _, goal := range []DeploymentGoal{"", "reset", "unknown"} {
		result, err := prepareApplicationDeployment(context.Background(), runner, goal, cycle)
		if err == nil || result.Outcome != "blocked" || result.Diagnostic == "" {
			t.Fatal("mutating deployment accepted invalid goal", goal, result, err)
		}
	}
	result, err := prepareApplicationDeployment(context.Background(), "relative", DeploymentUpdate, cycle)
	if err == nil || result.Outcome != "blocked" {
		t.Fatal("relative candidate accepted", result, err)
	}
	for _, goal := range []DeploymentGoal{DeploymentUpdate, DeploymentReplace} {
		result, err := prepareApplicationDeployment(context.Background(), runner, goal, cycle)
		require(t, err)
		if result.Contract != DeploymentPreparationContract || result.Home != HomeDir() || result.Goal != goal || result.Outcome != "prepared" || result.VerifierSHA256 != taskPayloadDigest([]byte("fixed staged bytes")) || result.VerifiedAt.IsZero() || result.Source != "no_original_host" || result.Facts.NativeResources != "unverified" || cycles != 0 {
			t.Fatal("empty component invented settlement or lost invocation binding", result)
		}
	}
	// A real current-instance failed receipt must not be turned into permission
	// by the new update orchestrator. Replace must not enter recovery at all.
	backend, _ := components(HomeDir())
	process, err := httpclient.CurrentProcessIdentity()
	require(t, err)
	process.Created = "already-exited-generation"
	process.Executable = canonicalPath(backend.exe)
	home, err := localservice.HomeID(HomeDir())
	require(t, err)
	id := contract.Identity{Protocol: contract.Protocol, HomeID: home, InstanceID: localservice.Token(), Process: process, Address: "127.0.0.1:18742", Ready: true}
	require(t, localservice.Write(backend.dir, "instance.json", id))
	require(t, localservice.Write(backend.dir, "launch.json", launchRecord{Attempt: localservice.Token(), Process: &id.Process}))
	require(t, localservice.Write(backend.dir, "shutdown.json", contract.Shutdown{Version: 2, Identity: id, Projects: []contract.ProjectShutdown{}, Clean: false, Diagnostic: "current native state unknown"}))
	for _, goal := range []DeploymentGoal{DeploymentUpdate, DeploymentReplace} {
		result, err := prepareApplicationDeployment(context.Background(), runner, goal, cycle)
		if err == nil || result.Outcome != "blocked" || result.Facts.NativeResources != "unverified" || cycles != 0 {
			t.Fatal("current failed receipt bypassed", goal, result, err)
		}
		if goal == DeploymentUpdate && !strings.Contains(err.Error(), "matching shutdown receipt") {
			t.Fatal("current failed receipt was not explicitly fenced", err)
		}
	}
	require(t, localservice.Write(backend.dir, "shutdown.json", contract.Shutdown{Version: 2, Identity: id, Projects: []contract.ProjectShutdown{}, Runner: &contract.RunnerShutdown{ResourcesSettled: true}, Clean: true}))
	result, err = prepareApplicationDeployment(context.Background(), runner, DeploymentUpdate, cycle)
	require(t, err)
	if result.Source != "normal_stop" || result.Normal == nil || result.Normal.Identity != id || result.Facts.NativeResources != "verified" || cycles != 0 {
		t.Fatal("normal stop lost its evidence", result)
	}
}

func TestDeploymentPreparationPackage(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"steerline.exe", "steerline-web.exe", "steerrunner.exe"} {
		require(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0600))
	}
	digest, err := deploymentPackageDigest(dir)
	require(t, err)
	var text strings.Builder
	for _, name := range []string{"steerline.exe", "steerline-web.exe", "steerrunner.exe"} {
		text.WriteString(name + ":" + taskPayloadDigest([]byte(name)) + "\n")
	}
	if digest != taskPayloadDigest([]byte(text.String())) {
		t.Fatal("package fingerprint not deterministic", digest)
	}
	require(t, os.WriteFile(filepath.Join(dir, "steerline-web.exe"), []byte("other Web"), 0600))
	changed, err := deploymentPackageDigest(dir)
	require(t, err)
	if changed == digest {
		t.Fatal("package ignored Web replacement")
	}
	require(t, os.Remove(filepath.Join(dir, "steerrunner.exe")))
	if _, err := deploymentPackageDigest(dir); err == nil {
		t.Fatal("partial package accepted")
	}
}
