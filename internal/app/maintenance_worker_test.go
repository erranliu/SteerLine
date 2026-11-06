package app

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	backend "github.com/erranliu/SteerLine/internal/execution/backend"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type firstMaterialReadHost struct {
	backend.ManagedProcessHost
	used    atomic.Bool
	delay   time.Duration
	failure error
}

func (h *firstMaterialReadHost) StartManaged(cmd *exec.Cmd) (backend.StopHandle, error) {
	if h.used.CompareAndSwap(false, true) {
		if h.failure != nil {
			return nil, h.failure
		}
		time.Sleep(h.delay)
	}
	return h.ManagedProcessHost.StartManaged(cmd)
}

func TestMaintenancePreparationSurvivesRequestCancellationAndSlowReads(t *testing.T) {
	m, store, cli := pmsFixture(t)
	git := m.config.Git
	m.config.Git.Executable = "unavailable-git"
	request, cancel := context.WithCancel(context.Background())
	id := strings.Repeat("a", 32)
	p, err := m.generateAll(request, id)
	cancel()
	if err != nil || p.Status != "queued" || cli.submits != 0 || len(store.materials) != 0 {
		t.Fatal(p, err)
	}
	if _, err = m.generateAll(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	m.config.Git = git
	// Reopen from the saved intent, as the application does after restart.
	reopened := newProjectMaintenance(m.config, store)
	reopened.tests = m.tests
	if _, err = reopened.check(context.Background()); err != nil {
		t.Fatal(err)
	}
	store.batch.Commit = reopened.current().Commit
	reopened.config.ProcessHost = &firstMaterialReadHost{ManagedProcessHost: m.config.ProcessHost, delay: 5100 * time.Millisecond}
	started := time.Now()
	if err = reopened.processPreparation(context.Background()); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) < 5*time.Second || store.materials["101"].Status != "accepted" || cli.submits != 1 {
		t.Fatal("slow preparation did not finish independently of request/actor budget", store.materials["101"], cli.submits)
	}
	if err = reopened.processPreparation(context.Background()); err != nil || cli.submits != 1 {
		t.Fatal("restart replay submitted twice", err)
	}
}

func TestMaintenancePreparationPersistsTimeoutAndContinuesOtherTasks(t *testing.T) {
	m, store, cli := pmsFixture(t)
	second := store.graph.Tasks[0]
	second.TaskID = "102"
	store.graph.Tasks = append(store.graph.Tasks, second)
	ctx := context.Background()
	if _, err := m.generateAll(ctx, strings.Repeat("b", 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := m.check(ctx); err != nil {
		t.Fatal(err)
	}
	// Only the material read fails; persistence still has the worker's live context.
	m.config.ProcessHost = &firstMaterialReadHost{ManagedProcessHost: m.config.ProcessHost, failure: context.DeadlineExceeded}
	store.batch.Commit = m.current().Commit
	if err := m.processPreparation(ctx); err != nil {
		t.Fatal(err)
	}
	if got := store.materials["101"]; got.Status != "pending" || !strings.Contains(got.Diagnostic, "deadline exceeded") {
		t.Fatal("timeout was not persisted", got)
	}
	if store.materials["102"].Status != "accepted" || cli.submits != 1 {
		t.Fatal("first task starved the rest", store.materials, cli.submits)
	}
	cli.finish()
	if err := m.processPreparation(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.processPreparation(ctx); err != nil || cli.submits != 1 {
		t.Fatal("failed material automatically retried", err)
	}
}

func TestMaintenanceCandidateAdmissionNeverGeneratesMaterials(t *testing.T) {
	m, store, cli := pmsFixture(t)
	_, _, err := m.prepareLatest(context.Background(), taskmodel.FixedWork{Record: store.graph.Tasks[0]}, false, func(context.Context, taskmodel.FixedWork, bool) (string, error) {
		t.Fatal("unprepared task reached base resolution")
		return "", nil
	})
	var rejected *taskmodel.Error
	if !errors.As(err, &rejected) || rejected.Code != "project_not_ready" || cli.submits != 0 || len(store.materials) != 0 {
		t.Fatal(err)
	}
	if err := m.processPreparation(context.Background()); err != nil || cli.submits != 0 || len(store.materials) != 0 {
		t.Fatal("idle worker prepared unregistered task", err)
	}
}

func TestMaintenancePreparationRejectsChangedQueuedProfile(t *testing.T) {
	m, store, cli := pmsFixture(t)
	if _, err := m.generateAll(context.Background(), strings.Repeat("c", 32)); err != nil {
		t.Fatal(err)
	}
	m.config.RunnerExecution.Profiles[0].Name = "renamed"
	// Names are presentation only; execution settings define the frozen profile.
	m.config.RunnerExecution.Profiles[0].Execution.Settings = []byte(`{"access_mode":"full_access","model":"different"}`)
	if err := m.processPreparation(context.Background()); err == nil || store.batch.Status != "failed" || cli.submits != 0 {
		t.Fatal("queued request changed execution profile", store.batch, err)
	}
}
