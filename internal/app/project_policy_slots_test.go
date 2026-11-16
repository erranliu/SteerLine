package app

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectPolicySlotsPinnedCatalogAndRequestBytes(t *testing.T) {
	m, store := pmsFixture(t)
	c := m.config
	slots, err := c.runtimeCatalog.ProjectPolicySlots()
	require(t, err)
	for _, slot := range slots {
		file := filepath.Join(c.Repository, filepath.FromSlash(slot.Path))
		require(t, os.MkdirAll(filepath.Dir(file), 0700))
		require(t, os.WriteFile(file, []byte("# Fixed project policy\n"), 0600))
	}
	pmGit(t, c.Repository, "add", ".")
	pmGit(t, c.Repository, "-c", "commit.gpgsign=false", "commit", "-qm", "project policies")
	commit := pmGit(t, c.Repository, "rev-parse", "HEAD")
	store.contract.SpecRef.Commit = commit
	store.contract.PlanRef.Commit = commit
	work := callerFrozenWork(t, m, store.contract)
	// The old constructor selected these same slots from the same fixed commit.
	before := callerTestRun()
	before.Repository.BaseRevision = commit
	before.Workflow.ProjectPolicySlots = []string{"git", "command", "testing"}
	_, err = c.prepareTestPlan(context.Background(), work, &before)
	require(t, err)
	after := callerTestRun()
	after.Repository.BaseRevision = commit
	service := runnerTaskService(c, nil, runnerclient.Client{}, nil)
	_, err = service.PrepareTestPlan(context.Background(), work, &after)
	require(t, err)
	oldBody, newBody := mustJSON(before), mustJSON(after)
	if !bytes.Equal(oldBody, newBody) {
		t.Fatal("published policy selection changed full request bytes")
	}
	t.Logf("full Run input UTF-8 bytes before=%d after=%d, identical", len(oldBody), len(newBody))
	for key, body := range before.Workflow.PromptOverrides {
		if after.Workflow.PromptOverrides[key] != body {
			t.Fatal("model-visible body changed", key)
		}
		t.Logf("model-visible %s UTF-8 bytes before=%d after=%d, identical", key, len(body), len(after.Workflow.PromptOverrides[key]))
	}
	// Dirty files cannot change the selection made from the pinned commit.
	require(t, os.Remove(filepath.Join(c.Repository, filepath.FromSlash(slots[0].Path))))
	selected, err := c.projectPolicySlots(context.Background(), commit)
	require(t, err)
	if len(selected) != 3 || selected[0] != "git" {
		t.Fatal(selected)
	}
	// Selection comes from the actual published descriptor, including its ordering.
	c.runtimeCatalog.Capabilities.ModelInputContract = json.RawMessage(`{"workflow":{"project_policies":{"slots":[{"slot":"custom","path":"task.spec.md"}]}}}`)
	selected, err = c.projectPolicySlots(context.Background(), commit)
	require(t, err)
	if len(selected) != 1 || selected[0] != "custom" {
		t.Fatal("caller retained a private slot list", selected)
	}
	c.runtimeCatalog.Capabilities.ModelInputContract = json.RawMessage(`{}`)
	if _, err = c.projectPolicySlots(context.Background(), commit); err == nil {
		t.Fatal("missing descriptor became an empty selection")
	}
}
