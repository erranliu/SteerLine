package console

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
)

func TestConsoleFollowsCurrentAttemptUntilHistoryIsSelected(t *testing.T) {
	run := &steerline.Run{TaskID: "task", DomainTaskID: "task", RunID: "run", ExecutionID: "execution", AttemptID: "first"}
	state := projectState{
		selectedKey: "run:run",
		snapshot: ProjectSnapshot{
			Observation: steerline.ObservationSnapshot{Runtime: &steerline.Runtime{Runs: []*steerline.Run{run}}},
			NativeTasks: &steerline.NativeTaskPage{Tasks: []steerline.NativeTaskDetail{{TaskRecord: steerline.TaskRecord{TaskID: "task"}}}},
		},
	}
	m := Model{projects: []projectState{state}}
	m, _ = m.refreshSelectedLogs(0)
	if m.projects[0].attemptID != "" || selectedAttempt(m.projects[0], run) != "first" {
		t.Fatal("initial current attempt was pinned instead of followed")
	}
	run.AttemptID = "second"
	m, _ = m.refreshSelectedLogs(0)
	if m.projects[0].attemptID != "" || selectedAttempt(m.projects[0], run) != "second" || !strings.Contains(m.projects[0].activityKey, "second") {
		t.Fatal("new current attempt did not replace the followed activity and detail")
	}
	created := time.Now().UTC()
	m.projects[0].snapshot.Domain = &steerline.DomainSnapshot{Attempts: []steerline.StepAttemptRecord{
		{ExecutionID: "execution", AttemptID: "first", AttemptNumber: 1, CreatedAt: created},
		{ExecutionID: "execution", AttemptID: "second", AttemptNumber: 2, CreatedAt: created.Add(time.Second)},
	}}
	updated, _ := m.cycleAttempt(true)
	m = updated.(Model)
	if m.projects[0].attemptID != "first" || selectedAttempt(m.projects[0], run) != "first" {
		t.Fatal("manual history selection did not pin the older attempt")
	}
	run.AttemptID = "third"
	m, _ = m.refreshSelectedLogs(0)
	if selectedAttempt(m.projects[0], run) != "first" {
		t.Fatal("a newer current attempt displaced the manually selected history")
	}
}

func TestConsoleRestoresOnlyExplicitHistoryAttemptPin(t *testing.T) {
	for _, pinned := range []bool{false, true} {
		t.Run(map[bool]string{false: "automatic-follow", true: "manual-history"}[pinned], func(t *testing.T) {
			t.Setenv("STEERLINE_HOME", t.TempDir())
			profile, prefs, _, err := acquireViewProfile()
			if err != nil {
				t.Fatal(err)
			}
			configPath := filepath.Join(t.TempDir(), "config.json")
			runDir := t.TempDir()
			ref := ProjectRef{Key: "project", ConfigPath: configPath, RunDir: runDir}
			prefs.Projects[ref.Key] = ProjectView{ConfigPath: configPath, RunDir: runDir, AttemptID: "old-attempt", AttemptPinned: pinned}
			if err := writeViewFile(profile.path(), prefs); err != nil {
				t.Fatal(err)
			}
			profile.close()
			model, err := NewModel(context.Background(), []ProjectRef{ref}, Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer model.profile.close()
			want := ""
			if pinned {
				want = "old-attempt"
			}
			if model.projects[0].attemptID != want {
				t.Fatalf("restored attempt=%q, want %q", model.projects[0].attemptID, want)
			}
		})
	}
}
