package console

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/erranliu/SteerLine/internal/steerline"
)

func TestReadyStartChoosesConfiguredProfileAndBindsRequest(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("profiles")
	ref.Config.TaskAuthority = "native"
	ref.Config.Host, ref.Config.Agent, ref.Config.Sandbox = "local", "codex", "workspace-write"
	other := steerline.ExecutionProfile{ID: "orca-local", Name: "Orca 本机", Host: "orca", Agent: "codex", Model: "chosen-model", ReasoningEffort: "xhigh", Sandbox: "workspace-write", Orca: steerline.OrcaConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "15m"}}
	ref.Config.ExecutionProfiles = []steerline.ExecutionProfile{other}
	control := &testControlClient{}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{readySnapshot(ref, now)}, control, now)
	m.profile = &viewProfile{dir: t.TempDir()}
	updated, cmd := m.startSelected()
	m = updated.(Model)
	if cmd != nil || m.profileChoice == nil || len(m.projects[0].actions) != 0 {
		t.Fatal("start submitted before an execution profile was chosen")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if view := m.View(); !strings.Contains(view, "Orca 本机") || !strings.Contains(view, "chosen-model") || !strings.Contains(view, "xhigh") {
		t.Fatalf("configured profile was not reviewable: %s", view)
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil || m.profileChoice != nil || len(m.projects[0].actions) != 1 {
		t.Fatal("confirmed profile did not create exactly one request")
	}
	result := cmd()
	updated, _ = m.Update(result)
	m = updated.(Model)
	var payload steerline.ControlPayload
	if err := json.Unmarshal(control.request.Payload, &payload); err != nil || payload.TaskID != "task-B" || payload.ExecutionProfileID != other.ID || payload.ExecutionProfileDigest != other.Digest() {
		t.Fatalf("start request changed the selected profile: %+v / %v", payload, err)
	}
	for _, action := range m.projects[0].actions {
		if !controlRequestMatchesAction(&steerline.ControlRequest{RequestID: action.RequestID, Action: action.Action, InstanceID: action.TargetInstance, RepositoryIdentity: ref.Identity, Payload: control.request.Payload}, action, ref.Identity) {
			t.Fatal("saved request did not retain the selected profile identity")
		}
	}
}

func TestRunDetailUsesItsFixedProfileAfterProjectConfigChanges(t *testing.T) {
	profile := steerline.ExecutionProfile{ID: "orca-remote", Name: "远程审查", Host: "orca_supervised", Agent: "codex", Model: "fixed-model", ReasoningEffort: "xhigh", Sandbox: "workspace-write"}
	fixed := profile.Freeze()
	run := &steerline.Run{RunID: "run-1", TaskID: "101", Backend: "orca", Agent: "codex"}
	state := projectState{ref: ProjectRef{Config: steerline.Config{Host: "local", Model: "new-project-default"}}, snapshot: ProjectSnapshot{Domain: &steerline.DomainSnapshot{WorkflowRuns: []steerline.WorkflowRunRecord{{WorkflowRunID: run.RunID, ExecutionProfile: &fixed}}}}}
	shown := strings.Join(renderFixedRunExecutionProfile(state, run), "\n")
	for _, want := range []string{"远程审查", "orca-remote", "Orca 受监管远程", "fixed-model", "xhigh"} {
		if !strings.Contains(shown, want) {
			t.Fatalf("run adopted current project defaults instead of fixed profile: %s", shown)
		}
	}
}

func TestLongExecutionProfileListKeepsSelectionVisible(t *testing.T) {
	ref := testProject("many")
	ref.Config.Host, ref.Config.Agent = "local", "codex"
	for i := 0; i < 28; i++ {
		ref.Config.ExecutionProfiles = append(ref.Config.ExecutionProfiles, steerline.ExecutionProfile{ID: "profile-" + strconv.Itoa(i), Name: "方案 " + strconv.Itoa(i), Host: "local", Agent: "codex", Model: "model-" + strconv.Itoa(i), Sandbox: "workspace-write"})
	}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{readySnapshot(ref, time.Now().UTC())}, &testControlClient{}, time.Now().UTC())
	m.width, m.height = 80, 24
	m.profileChoice = &executionProfileChoice{projectKey: ref.Key, taskID: "task-B", index: 28}
	view := m.View()
	if !strings.Contains(view, "profile-27") || !strings.Contains(view, "model-27") || len(strings.Split(view, "\n")) != 24 {
		t.Fatalf("selected profile fell outside the narrow viewport: %s", view)
	}
}
