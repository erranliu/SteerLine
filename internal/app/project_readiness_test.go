package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestProjectReadinessGatesAllUnfinishedTasksAndNewCandidates(t *testing.T) {
	ctx := context.Background()
	m, store := pmsFixture(t)
	_, err := m.check(ctx)
	require(t, err)
	require(t, m.refresh(ctx))
	require(t, m.refresh(ctx))
	first := store.materials["101"]
	other := first
	other.TaskID = "102"
	store.graph.Tasks = append(store.graph.Tasks, taskmodel.TaskRecord{TaskID: "102", Status: "open"})
	store.materials["102"] = other
	work := taskmodel.FixedWork{Record: store.graph.Tasks[0], Contract: store.contract}
	port := &runtimeProjectPort{readiness: m}
	baseCalls := 0
	base := func(context.Context, taskmodel.FixedWork, string) (string, error) {
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
		if readiness.Blockers == nil || maintenance.Readiness.Blockers == nil ||
			(readiness.Status == "ready") != ready || maintenance.Readiness.Status != readiness.Status ||
			!slices.Equal(maintenance.Readiness.Blockers, readiness.Blockers) {
			t.Fatalf("maintenance and admission disagree: %+v %+v", readiness, maintenance.Readiness)
		}
		before := baseCalls
		_, _, err = m.prepareLatest(ctx, work, m.current().Commit, base)
		if ready {
			require(t, err)
		} else {
			var rejected *taskmodel.Error
			if !errors.As(err, &rejected) || rejected.Code != "project_not_ready" || baseCalls != before {
				t.Fatalf("unready project reached base/candidate creation: %v", err)
			}
		}
	}
	assert(true)
	for _, status := range []string{"unprepared", "pending", "accepted", "failed", "invalid", "cancelled", "configuration_required"} {
		other.Status = status
		store.materials["102"] = other
		assert(false)
	}
	delete(store.materials, "102")
	// Missing observation is not ready until the finite Git reader reconstructs it.
	projection, err := m.projectReadiness(ctx)
	require(t, err)
	if projection.Status != "not_ready" {
		t.Fatal(projection)
	}
	require(t, m.refresh(ctx))
	assert(true)
	other = first
	other.TaskID, other.Commit = "102", strings.Repeat("0", 40)
	store.materials["102"] = other
	projection, err = m.projectReadiness(ctx)
	require(t, err)
	if projection.Status != "not_ready" {
		t.Fatal(projection)
	}
	require(t, m.refresh(ctx))
	assert(true)
	// A truly invalid unfinished sibling still blocks; closed ones are excluded.
	other.Status = "invalid"
	store.materials["102"] = other
	// Closed tasks do not retain a project-wide blocker.
	for _, status := range []string{"closed", "discarded"} {
		store.graph.Tasks[1].Status = status
		assert(true)
	}
	store.graph.Tasks[1].Status = "in_progress"
	other.Commit = first.Commit
	other.Status = "ready"
	store.materials["102"] = other
	assert(true)
}

func TestProjectReadinessRequiresSafeLocalSynchronizationAndRechecksReservation(t *testing.T) {
	ctx := context.Background()
	m, store := pmsFixture(t)
	_, err := m.check(ctx)
	require(t, err)
	require(t, m.refresh(ctx))
	require(t, m.refresh(ctx))
	commit := m.current().Commit
	reserved := 0
	save := func() error { reserved++; return nil }
	require(t, m.reserveReadyExecution(ctx, commit, save))
	checkBlocked := func(codes ...pm.BlockerCode) {
		t.Helper()
		readiness, err := m.projectReadiness(ctx)
		require(t, err)
		if readiness.Status != "not_ready" || len(readiness.Blockers) == 0 {
			t.Fatal(readiness)
		}
		for _, code := range codes {
			if !slices.ContainsFunc(readiness.Blockers, func(blocker pm.Blocker) bool { return blocker.Code == code }) {
				t.Fatalf("missing %s in %+v", code, readiness.Blockers)
			}
		}
		if err := m.reserveReadyExecution(ctx, commit, save); err == nil || reserved != 1 {
			t.Fatal("changed project admitted a reservation", err, reserved)
		}
	}
	dirty := filepath.Join(m.config.Repository, "untracked.txt")
	require(t, os.WriteFile(dirty, []byte("work"), 0600))
	checkBlocked(pm.WorktreeDirty)
	require(t, os.Remove(dirty))
	pmGit(t, m.config.Repository, "checkout", "--detach", commit)
	checkBlocked(pm.DetachedHEAD)
	pmGit(t, m.config.Repository, "checkout", "-b", "different")
	checkBlocked(pm.WrongBranch)
	pmGit(t, m.config.Repository, "checkout", "main")
	pmGit(t, m.config.Repository, "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "ahead")
	checkBlocked(pm.LocalAheadRemote)
	pmGit(t, m.config.Repository, "push", "origin", "main")
	newCommit := pmGit(t, m.config.Repository, "rev-parse", "HEAD")
	pmGit(t, m.config.Repository, "checkout", "--detach", commit)
	pmGit(t, m.config.Repository, "branch", "-f", "main", commit)
	pmGit(t, m.config.Repository, "checkout", "main")
	_, err = m.check(ctx)
	require(t, err)
	require(t, os.WriteFile(dirty, []byte("work"), 0600))
	checkBlocked(pm.WorktreeDirty, pm.LocalBehindRemote)
	require(t, os.Remove(dirty))
	pmGit(t, m.config.Repository, "merge", "--ff-only", newCommit)
	// Unchanged Git arrangements remain ready after the baseline advances.
	require(t, m.refresh(ctx))
	ready, err := m.projectReadiness(ctx)
	require(t, err)
	if ready.Status != "ready" {
		t.Fatal(ready)
	}
	if err := m.reserveReadyExecution(ctx, commit, save); err == nil || reserved != 1 {
		t.Fatal("ready project admitted a reservation at the superseded baseline", err, reserved)
	}
	require(t, m.reserveReadyExecution(ctx, newCommit, func() error { return nil }))
	m.observation.Diagnostic = "remote unavailable"
	checkBlocked(pm.RemoteCheckFailed)
	m.observation.Diagnostic = ""
	material := store.materials["101"]
	material.Status = "failed"
	store.materials["101"] = material
	checkBlocked(pm.TaskMaterialInvalid)
}

func TestMaintenanceCandidateAdmissionNeverGeneratesMaterials(t *testing.T) {
	m, store := pmsFixture(t)
	pmGit(t, m.config.Repository, "rm", "task.tests.json")
	pmGit(t, m.config.Repository, "-c", "commit.gpgsign=false", "commit", "-qm", "missing arrangement")
	pmGit(t, m.config.Repository, "push", "origin", "main")
	ctx := t.Context()
	observed, err := m.check(ctx)
	require(t, err)
	_, _, err = m.prepareLatest(ctx, taskmodel.FixedWork{Record: store.graph.Tasks[0]}, observed.Commit, func(context.Context, taskmodel.FixedWork, string) (string, error) {
		t.Fatal("invalid material reached base")
		return "", nil
	})
	var rejected *taskmodel.Error
	if !errors.As(err, &rejected) || rejected.Code != "project_not_ready" || store.materials["101"].Status != "invalid" {
		t.Fatal(err, store.materials)
	}
	readiness, err := m.projectReadiness(context.Background())
	require(t, err)
	want := pm.Blocker{Category: pm.TestPlan, Code: pm.TestPlanMissing, TaskID: "101", Path: "task.tests.json"}
	if !slices.ContainsFunc(readiness.Blockers, func(blocker pm.Blocker) bool {
		blocker.Diagnostic = ""
		return blocker == want
	}) {
		t.Fatalf("missing committed arrangement reason: %+v", readiness.Blockers)
	}
}
