package app

import (
	"context"
	"encoding/json"
	host "github.com/erranliu/SteerLine/internal/app/host"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUnifiedStartupLifecycleSharedPIDIndependentInstances(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-fixture")
	require(t, os.MkdirAll(EtcDir(), 0700))
	require(t, os.WriteFile(filepath.Join(EtcDir(), "one.json"), projectConfigJSON(t, c), 0600))
	second := c
	second.Repository = filepath.Join(t.TempDir(), "second")
	fixtureGit(t, HomeDir(), "clone", "--quiet", "--no-hardlinks", c.Repository, second.Repository)
	fixtureGit(t, second.Repository, "remote", "set-url", "origin", "https://github.com/owner/repo.git")
	second.RunDir = ""
	require(t, os.WriteFile(filepath.Join(EtcDir(), "two.json"), projectConfigJSON(t, second), 0600))
	require(t, os.WriteFile(filepath.Join(EtcDir(), "z-broken.json"), []byte("{"), 0600))
	d := NewApplicationDirectory(HomeDir())
	defer d.Close()
	d.Start()
	catalog := switchCatalog(t, d, func(c contract.Catalog) bool {
		return len(c.Projects) == 3 && (c.Projects[0].RuntimeState != "starting" && c.Projects[1].RuntimeState != "starting")
	})
	for _, project := range catalog.Projects {
		if project.ConfigID == "z-broken.json" {
			if project.RuntimeState != "failed" || project.ProjectID != "" {
				t.Fatal("bad configuration was not isolated", project)
			}
			continue
		}
		if !project.Online {
			t.Fatal("project failed", project)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a, err := d.Connect(ctx, catalog.Projects[0].ProjectID)
	require(t, err)
	b, err := d.Connect(ctx, catalog.Projects[1].ProjectID)
	require(t, err)
	readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
	defer readCancel()
	if _, err := a.Read(readCtx, "/identity", nil); err != nil {
		t.Fatal("in-process identity read failed", err)
	}
	ai, bi := a.Identity(ctx), b.Identity(ctx)
	if ai.InstanceID == bi.InstanceID || ai.ProjectID == bi.ProjectID || ai.Process != bi.Process || ai.Process.PID != os.Getpid() {
		t.Fatal("project/process ownership conflated", ai, bi)
	}
	for _, identity := range []httpcontract.Identity{ai, bi} {
		for _, capability := range identity.Capabilities {
			if capability == "stop_service" {
				t.Fatal("retired capability exposed")
			}
		}
	}
	if _, err = a.Command(ctx, httpcontract.CommandEnvelope{RequestID: strings.Repeat("f", 32), ProjectID: ai.ProjectID, InstanceID: ai.InstanceID, Action: "stop_service", TargetID: ai.InstanceID, Payload: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("retired project stop accepted")
	}
	if !b.Identity(ctx).Ready {
		t.Fatal("retired stop affected sibling runtime")
	}
	for _, e := range d.entries {
		if e.ref.Identity == "" {
			continue
		} // Invalid configs own no runtime or repository lock.
		if _, err = os.Stat(filepath.Join(e.ref.RunDir, "local-api", "discovery.json")); !os.IsNotExist(err) {
			t.Fatal("per-project HTTP published")
		}
		lock, e := host.LockRepository(e.ref.Identity)
		if e == nil {
			lock.Close()
			t.Fatal("repository lock was not retained")
		}
	}
	d.BeginStop()
	require(t, d.Close())
	for _, result := range d.ShutdownResults() {
		if !result.LocalClosed || !result.LocksReleased || !result.ResourcesSettled {
			t.Fatal("project not cleanly closed", result)
		}
	}
	if a.Identity(ctx).Ready || b.Identity(ctx).Ready {
		t.Fatal("closed project remains ready")
	}
}
