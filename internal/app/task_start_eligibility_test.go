package app

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/app/host"
	executionprocess "github.com/erranliu/SteerLine/internal/execution/process"
	"github.com/erranliu/SteerLine/internal/hostcontract"
	"github.com/erranliu/SteerLine/internal/product/api"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskapp "github.com/erranliu/SteerLine/internal/task"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

type readinessQueryStub struct {
	value pm.Readiness
	err   error
	calls int
}

func (s *readinessQueryStub) projectReadiness(context.Context) (pm.Readiness, error) {
	s.calls++
	return s.value, s.err
}

type dispatchGraphStore struct {
	taskrunner.ActorStore
	graph ports.TaskGraphSnapshot
}

func (s dispatchGraphStore) TaskGraphSnapshot(context.Context, string) (ports.TaskGraphSnapshot, error) {
	return s.graph, nil
}

func TestTaskStartAndDispatchUseInjectedReadiness(t *testing.T) {
	ctx := t.Context()
	task := taskmodel.TaskRecord{TaskID: "1", Status: "open", Type: "task"}
	store := dispatchGraphStore{graph: ports.TaskGraphSnapshot{Authority: taskmodel.ProjectTaskAuthority{DispatchMode: "auto"}, Tasks: []taskmodel.TaskRecord{task}}}
	const profile = "default"
	environmentChecks := 0
	service := &taskrunner.Service{Project: "fixture", Store: store, DefaultProfile: profile,
		Tasks:    taskapp.Service{Input: taskapp.Input{ExecutableTypes: []string{"task"}}},
		Profiles: map[string]taskrunner.Profile{profile: {Input: runnerclient.RunInput{Workflow: runnercontract.Workflow{ID: "development", Version: 2}}}},
		CheckExecution: func(_ context.Context, _ string, refresh bool) error {
			if refresh {
				t.Fatal("read-only status refreshed execution admission")
			}
			environmentChecks++
			return nil
		},
	}
	actor, err := taskrunner.NewActor(taskrunner.ActorOptions{Tasks: service, Store: store,
		Host:       host.NewServiceHost(host.Config{RunDir: t.TempDir(), ProcessFacts: executionprocess.NewServiceFacts()}, "off", io.Discard, nil),
		Repository: hostcontract.Repository{Identity: service.Project}, Mode: "manual", Heartbeat: time.Second,
		Control: func(context.Context, api.ControlRequest) (api.ControlReceipt, error) {
			return api.ControlReceipt{}, nil
		},
	})
	require(t, err)
	config := Config{RunnerExecution: RunnerExecutionConfig{Profiles: []RunnerProfile{{ID: profile}}}}
	readiness := &readinessQueryStub{}
	reader := &clientReader{config: config, executableTypes: []string{"task"}, startActor: actor, readiness: readiness}
	port := &runtimeProjectPort{config: config, actor: actor, readiness: readiness}

	reader.taskStartEligibility(ctx, "auto", ports.NativeTaskDetail{TaskRecord: task})
	if readiness.calls != 0 {
		t.Fatal("task-level blocker should short-circuit readiness")
	}
	for _, tc := range []struct {
		status string
		err    error
		code   string
	}{
		{status: "not_ready", code: "project_not_ready"},
		{err: errors.New("read failed"), code: "project_readiness_unverified"},
		{status: "ready"},
	} {
		readiness.value, readiness.err = pm.Readiness{Status: tc.status}, tc.err
		start := reader.taskStartEligibility(ctx, "manual", ports.NativeTaskDetail{TaskRecord: task})
		dispatch, err := port.readDispatch(ctx)
		require(t, err)
		if tc.code == "" {
			if len(start.Blockers) != 0 || len(start.ProfileBlockers) != 1 || start.ProfileBlockers[profile] != nil || dispatch.State != "ready" || dispatch.Blocker != nil {
				t.Fatalf("ready observation: start=%+v dispatch=%+v", start, dispatch)
			}
		} else if len(start.Blockers) != 1 || start.Blockers[0].Code != tc.code || dispatch.State != "waiting" || dispatch.Blocker == nil || dispatch.Blocker.Category != "project" || dispatch.Blocker.Code != tc.code {
			t.Fatalf("expected %s: start=%+v dispatch=%+v", tc.code, start, dispatch)
		}
	}
	if readiness.calls != 6 || environmentChecks != 2 {
		t.Fatalf("readiness calls = %d, environment checks = %d", readiness.calls, environmentChecks)
	}
}

func TestTaskStartEligibility(t *testing.T) {
	for _, test := range []struct {
		name, status, mode, action, code string
		active                           int
		merged                           bool
	}{
		{name: "manual first start", status: "open", mode: "manual", action: "start_task"},
		{name: "auto first start", status: "open", mode: "auto", action: "start_task", code: "wrong_mode"},
		{name: "explicit candidate in auto mode", status: "in_progress", mode: "auto", action: "start_candidate", active: 4},
		{name: "candidate limit", status: "in_progress", mode: "manual", action: "start_candidate", active: 5, code: "candidate_limit_reached"},
		{name: "observed merge", status: "in_progress", mode: "manual", action: "start_candidate", merged: true, code: "candidate_pr_merged"},
		{name: "discarded task", status: "discarded", mode: "manual", action: "start_task", code: "task_discarded"},
	} {
		t.Run(test.name, func(t *testing.T) {
			view := taskStartEligibility(test.mode, ports.NativeTaskDetail{
				TaskRecord: taskmodel.TaskRecord{Status: test.status}, CurrentMaterialCandidates: test.active, ObservedMergedCandidate: test.merged,
			}, true)
			if view.Action != test.action {
				t.Fatalf("action = %s, want %s", view.Action, test.action)
			}
			if test.code == "" {
				if len(view.Blockers) != 0 {
					t.Fatalf("unexpected blockers: %+v", view.Blockers)
				}
			} else if len(view.Blockers) != 1 || view.Blockers[0].Code != test.code || view.Blockers[0].Diagnostic == "" {
				t.Fatalf("expected structured %s blocker, got %+v", test.code, view.Blockers)
			}
		})
	}
}
