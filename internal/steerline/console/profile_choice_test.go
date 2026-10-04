package console

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

func consoleLocalProfile(id, name, model, effort string) api.ExecutionProfileSummary {
	return consoleFixtureProfile(id, name, "local", "codex", []string{"Model：" + model, "Reasoning effort：" + effort})
}

func TestReadyStartChoosesConfiguredProfileAndBindsRequest(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("profiles")
	ref.Presentation.TaskAuthority = "native"
	ref.Presentation.WorkflowProtocol = fixtureWorkflowVersion
	other := consoleLocalProfile("local-strong", "本地 Codex 高强度", "chosen-model", "xhigh")
	ref.Presentation.Profiles = append(ref.Presentation.Profiles, other)
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
	if view := m.View(); !strings.Contains(view, "本地 Codex 高强度") || !strings.Contains(view, "chosen-model") || !strings.Contains(view, "xhigh") {
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
	var payload productapi.ControlPayload
	if err := json.Unmarshal(control.request.Payload, &payload); err != nil || payload.TaskID != "task-B" || payload.ExecutionProfileID != other.ID || payload.ExecutionProfileDigest != other.Digest {
		t.Fatalf("start request changed the selected profile: %+v / %v", payload, err)
	}
	for _, action := range m.projects[0].actions {
		if !controlRequestMatchesAction(&productapi.ControlRequest{RequestID: action.RequestID, Action: action.Action, InstanceID: action.TargetInstance, RepositoryIdentity: ref.Identity, Payload: control.request.Payload}, action, ref.Identity) {
			t.Fatal("saved request did not retain the selected profile identity")
		}
	}
}

func TestRunDetailUsesItsFixedProfileAfterProjectConfigChanges(t *testing.T) {
	fixed := executionmodel.FixedExecutionProfile{ID: "orca-remote", Name: "远程审查", Host: "orca_supervised", Agent: "codex", ImplementationDisplay: []string{"Model：fixed-model", "Reasoning effort：xhigh", "目标环境：remote-env", "连接：fixed-connection"}, PublicationPolicy: executionbackend.PublicationPolicy{Publisher: "worker"}, Publication: executionmodel.PublicationSettings{}}
	fixed.Publication.Provider.Kind, fixed.Publication.Provider.BaseURL, fixed.Publication.Provider.Repository = "gitea", "https://fixed.invalid", "org/repo"
	fixed.Publication.Remote, fixed.Publication.RemoteURL, fixed.Publication.BaseBranch = "origin", "ssh://git@fixed.invalid/org/repo.git", "main"
	fixed.Publication.AuthenticationRefs = []string{"FIXED_TOKEN"}
	run := &contracts.Run{RunID: "run-1", TaskID: "101", Backend: "orca", Agent: "codex"}
	state := projectState{ref: ProjectRef{Presentation: api.ProjectPresentation{Profiles: []api.ExecutionProfileSummary{{Host: "local", Agent: "agent", ImplementationDisplay: []string{"Model：new-project-default"}}}}}, snapshot: ProjectSnapshot{Domain: &contracts.DomainSnapshot{WorkflowRuns: []executionmodel.WorkflowRunRecord{{WorkflowRunID: run.RunID, ExecutionProfile: &fixed}}}}}
	shown := strings.Join(renderFixedRunExecutionProfile(state, run), "\n")
	for _, want := range []string{"远程审查", "orca-remote", "orca_supervised", "fixed-model", "xhigh", "发布责任：worker", "https://fixed.invalid/org/repo", "ssh://git@fixed.invalid/org/repo.git", "FIXED_TOKEN"} {
		if !strings.Contains(shown, want) {
			t.Fatalf("run adopted current project defaults instead of fixed profile: %s", shown)
		}
	}
}

func TestLongExecutionProfileListKeepsSelectionVisible(t *testing.T) {
	ref := testProject("many")
	for i := 0; i < 28; i++ {
		ref.Presentation.Profiles = append(ref.Presentation.Profiles, consoleLocalProfile("profile-"+strconv.Itoa(i), "方案 "+strconv.Itoa(i), "model-"+strconv.Itoa(i), "high"))
	}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{readySnapshot(ref, time.Now().UTC())}, &testControlClient{}, time.Now().UTC())
	m.width, m.height = 80, 24
	m.profileChoice = &executionProfileChoice{projectKey: ref.Key, taskID: "task-B", index: 28}
	view := m.View()
	if !strings.Contains(view, "profile-27") || !strings.Contains(view, "model-27") || len(strings.Split(view, "\n")) != 24 {
		t.Fatalf("selected profile fell outside the narrow viewport: %s", view)
	}
}
