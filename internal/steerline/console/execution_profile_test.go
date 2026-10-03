package console

import (
	"strings"
	"testing"
	"time"

	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
)

func TestExecutionSettingsOnlyAppearInStartChoiceAndConfigMismatchBlocksStart(t *testing.T) {
	ref := testProject("native")
	ref.Identity = "identity"
	now := time.Now().UTC()
	snapshot := readySnapshot(ref, now)
	snapshot.Connection.Snapshot.Runtime.Service.ConfigDigest = ref.Presentation.ConfigDigest
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, &testControlClient{}, now)
	mainDetail := strings.Join(renderDetail(m, m.projects[0], m.selectedRow(0), 120, 100), "\n")
	if strings.Contains(mainDetail, "Agent：") || strings.Contains(mainDetail, "执行配置：") {
		t.Fatalf("execution settings appeared before s: %q", mainDetail)
	}
	m.profileChoice = &executionProfileChoice{taskID: "task-B"}
	current := renderExecutionProfileChoice(m, m.projects[0], 40)
	for _, want := range []string{"local", "Agent：codex", "未在全局配置固定", "Reasoning effort：high", "Sandbox：workspace-write"} {
		if !strings.Contains(current, want) {
			t.Fatalf("ready task missed %q: %s", want, current)
		}
	}
	snapshot.Connection.Snapshot.Runtime.Service.ConfigDigest = "different-loaded-config"
	if !executionConfigMismatch(ref, snapshot) || !strings.Contains(api.StartTaskReason(ref, snapshot, now), "配置不同") {
		t.Fatal("a mismatched service config remained startable")
	}
	m = modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, &testControlClient{}, now)
	updated, cmd := m.startSelected()
	m = updated.(Model)
	if cmd != nil || m.projects[0].client.(*testClient).control.calls != 0 || !strings.Contains(m.status, "不能开始") {
		t.Fatalf("mismatched config submitted a task: %q", m.status)
	}
	m.profileChoice = &executionProfileChoice{taskID: "task-B"}
	if detail := renderExecutionProfileChoice(m, m.projects[0], 40); !strings.Contains(detail, "配置不同") {
		t.Fatalf("mismatch exposed a guessed execution profile: %q", detail)
	}
	updated, cmd = m.toggleMode()
	m = updated.(Model)
	if cmd != nil || !strings.Contains(m.status, "不能切换自动派发") {
		t.Fatalf("mismatched config enabled auto dispatch: %q", m.status)
	}
}

func TestOrcaExecutionProfilesNameAgentModelEffortAndRemoteTarget(t *testing.T) {
	for _, sample := range []struct {
		config api.ExecutionProfileSummary
		want   []string
	}{
		{config: api.ExecutionProfileSummary{Host: "orca", Agent: "pi", ImplementationDisplay: []string{"Model：由所选 agent 决定", "Reasoning effort：不适用"}}, want: []string{"orca", "Agent：pi", "Model：由所选 agent 决定", "Reasoning effort：不适用"}},
		{config: api.ExecutionProfileSummary{Host: "orca", Agent: "codex", ImplementationDisplay: []string{"Model：chosen-model", "Reasoning effort：xhigh"}}, want: []string{"orca", "Agent：codex", "Model：chosen-model", "Reasoning effort：xhigh"}},
		{config: api.ExecutionProfileSummary{Host: "orca_supervised", Agent: "codex", ImplementationDisplay: []string{"Model：remote-model", "Reasoning effort：high", "目标环境：remote-env", "连接：fixed-connection"}}, want: []string{"orca_supervised", "Model：remote-model", "Reasoning effort：high", "目标环境：remote-env", "连接：fixed-connection"}},
	} {
		ref := ProjectRef{Presentation: api.ProjectPresentation{Profiles: []api.ExecutionProfileSummary{sample.config}}}
		m := Model{width: 120, profileChoice: &executionProfileChoice{taskID: "task"}}
		shown := renderExecutionProfileChoice(m, projectState{ref: ref}, 40)
		for _, want := range sample.want {
			if !strings.Contains(shown, want) {
				t.Fatalf("execution profile missed %q: %s", want, shown)
			}
		}
	}
}
