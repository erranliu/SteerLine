package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestProjectReadinessGatesAllUnfinishedTasksAndNewCandidates(t *testing.T) {
	ctx := context.Background()
	m, store, cli := pmsFixture(t)
	_, err := m.check(ctx)
	require(t, err)
	require(t, m.refresh(ctx))
	cli.finish()
	require(t, m.tests.service.Observe(ctx))
	require(t, m.refresh(ctx))
	first := store.materials["101"]
	other := first
	other.TaskID = "102"
	store.graph.Tasks = append(store.graph.Tasks, taskmodel.TaskRecord{TaskID: "102", Status: "open"})
	store.materials["102"] = other
	work := taskmodel.FixedWork{Record: store.graph.Tasks[0], Contract: store.contract}
	port := &runtimeProjectPort{config: Config{runtimeMaintenance: m}}
	baseCalls := 0
	base := func(context.Context, taskmodel.FixedWork, bool) (string, error) {
		baseCalls++
		return m.current().Commit, nil
	}
	assert := func(ready bool) {
		t.Helper()
		value, err := port.Read(ctx, "/maintenance/readiness", url.Values{})
		require(t, err)
		readiness := value.(pm.Readiness)
		view, err := m.baselineView(ctx)
		require(t, err)
		raw, err := json.Marshal(view)
		require(t, err)
		var maintenance struct {
			Readiness pm.Readiness `json:"readiness"`
		}
		require(t, json.Unmarshal(raw, &maintenance))
		if (readiness.Status == "ready") != ready || maintenance.Readiness.Status != readiness.Status {
			t.Fatalf("maintenance and admission disagree: %+v %+v", readiness, maintenance.Readiness)
		}
		for _, candidate := range []bool{false, true} {
			before := baseCalls
			_, _, err := m.prepare(ctx, work, candidate, base)
			if ready {
				require(t, err)
			} else {
				var rejected *taskmodel.Error
				if !errors.As(err, &rejected) || rejected.Code != "project_not_ready" || baseCalls != before {
					t.Fatalf("unready project reached base/candidate creation: %v", err)
				}
			}
		}
	}
	assert(true)
	for _, status := range []string{"unprepared", "pending", "accepted", "failed", "unknown", "invalid", "cancelled", "configuration_required"} {
		other.Status = status
		store.materials["102"] = other
		assert(false)
	}
	delete(store.materials, "102")
	assert(false)
	other = first
	other.TaskID, other.Commit = "102", strings.Repeat("0", 40)
	store.materials["102"] = other
	assert(false)
	// Closed tasks do not retain a project-wide blocker.
	for _, status := range []string{"closed", "discarded"} {
		store.graph.Tasks[1].Status = status
		assert(true)
	}
	store.graph.Tasks[1].Status = "in_progress"
	other.Commit = first.Commit
	store.materials["102"] = other
	assert(true)
	// A cancelled generation invalidates even a not-yet-refreshed ready material.
	g := store.generations[first.GenerationID]
	g.Cancelled = true
	store.generations[g.ID] = g
	assert(false)
	g.Cancelled = false
	store.generations[g.ID] = g
	assert(true)
	if cli.submits != 1 {
		t.Fatal("readiness or admission generated new tests", cli.submits)
	}
}

func TestProjectReadinessRequiresSafeLocalSynchronizationAndRechecksReservation(t *testing.T) {
	ctx := context.Background()
	m, store, cli := pmsFixture(t)
	_, err := m.check(ctx)
	require(t, err)
	require(t, m.refresh(ctx))
	cli.finish()
	require(t, m.tests.service.Observe(ctx))
	require(t, m.refresh(ctx))
	commit := m.current().Commit
	reserved := 0
	save := func() error { reserved++; return nil }
	require(t, m.reserveReadyExecution(ctx, commit, save))
	checkBlocked := func() {
		t.Helper()
		readiness, err := m.projectReadiness(ctx)
		require(t, err)
		if readiness.Status != "not_ready" || len(readiness.Blockers) == 0 {
			t.Fatal(readiness)
		}
		if err := m.reserveReadyExecution(ctx, commit, save); err == nil || reserved != 1 {
			t.Fatal("changed project admitted a reservation", err, reserved)
		}
	}
	dirty := filepath.Join(m.config.Repository, "untracked.txt")
	require(t, os.WriteFile(dirty, []byte("work"), 0600))
	checkBlocked()
	require(t, os.Remove(dirty))
	pmGit(t, m.config.Repository, "checkout", "--detach", commit)
	checkBlocked()
	pmGit(t, m.config.Repository, "checkout", "-b", "different")
	checkBlocked()
	pmGit(t, m.config.Repository, "checkout", "main")
	pmGit(t, m.config.Repository, "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "ahead")
	checkBlocked()
	pmGit(t, m.config.Repository, "push", "origin", "main")
	newCommit := pmGit(t, m.config.Repository, "rev-parse", "HEAD")
	pmGit(t, m.config.Repository, "checkout", "--detach", commit)
	pmGit(t, m.config.Repository, "branch", "-f", "main", commit)
	pmGit(t, m.config.Repository, "checkout", "main")
	_, err = m.check(ctx)
	require(t, err)
	checkBlocked()
	pmGit(t, m.config.Repository, "merge", "--ff-only", newCommit)
	// Existing generation is reused for unchanged materials after explicit prep.
	require(t, m.refresh(ctx))
	ready, err := m.projectReadiness(ctx)
	require(t, err)
	if ready.Status != "ready" || cli.submits != 1 {
		t.Fatal(ready, cli.submits)
	}
	m.observation.Diagnostic = "remote unavailable"
	checkBlocked()
	m.observation.Diagnostic = ""
	material := store.materials["101"]
	material.Status = "failed"
	store.materials["101"] = material
	checkBlocked()
}
