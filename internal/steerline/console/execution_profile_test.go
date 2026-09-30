package console

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
)

func TestExecutionSettingsOnlyAppearInStartChoiceAndConfigMismatchBlocksStart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STEERLINE_HOME", home)
	global, err := json.Marshal(map[string]any{"execution_profiles": []map[string]any{{"id": "local", "name": "Local Codex", "host": "local", "agent": "codex", "sandbox": "workspace-write", "codex": map[string]string{"executable": "codex"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(steerline.GlobalExecutionConfigPath(), global, 0600); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", "--quiet", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	path := filepath.Join(t.TempDir(), "project.json")
	data, err := json.Marshal(map[string]string{"repository": repo, "branch": "main", "task_authority": "native", "workflow_protocol": steerline.WorkerFlowVersion})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := steerline.LoadConfig(path)
	if err != nil || cfg.LoadedConfigDigest() == "" {
		t.Fatalf("loaded config has no digest: %v", err)
	}
	ref := testProject("native")
	ref.Config, ref.ConfigPath = cfg, path
	ref.Identity = "identity"
	now := time.Now().UTC()
	snapshot := readySnapshot(ref, now)
	snapshot.Connection.Snapshot.Runtime.Service.ConfigDigest = cfg.LoadedConfigDigest()
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, &testControlClient{}, now)
	mainDetail := strings.Join(renderDetail(m, m.projects[0], m.selectedRow(0), 120, 100), "\n")
	if strings.Contains(mainDetail, "Agent：") || strings.Contains(mainDetail, "执行配置：") {
		t.Fatalf("execution settings appeared before s: %q", mainDetail)
	}
	m.profileChoice = &executionProfileChoice{taskID: "task-B"}
	current := renderExecutionProfileChoice(m, m.projects[0], 40)
	for _, want := range []string{"本地 Codex", "Agent：codex", "未在全局配置固定", "Thinking effort：high", "Sandbox：workspace-write"} {
		if !strings.Contains(current, want) {
			t.Fatalf("ready task missed %q: %s", want, current)
		}
	}
	snapshot.Connection.Snapshot.Runtime.Service.ConfigDigest = "different-loaded-config"
	if !executionConfigMismatch(ref, snapshot) || !strings.Contains(candidateActionReason(ref, snapshot, now), "配置不同") {
		t.Fatal("a mismatched service config remained startable")
	}
	m = modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, &testControlClient{}, now)
	updated, cmd := m.startSelected()
	m = updated.(Model)
	if cmd != nil || m.control.(*testControlClient).calls != 0 || !strings.Contains(m.status, "不能开始") {
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
		config steerline.Config
		want   []string
	}{
		{config: steerline.Config{TaskAuthority: "native", Host: "orca", Agent: "pi", Sandbox: "workspace-write"}, want: []string{"Orca 本机终端", "Agent：pi", "Model：由所选 agent 决定", "Thinking effort：不适用"}},
		{config: steerline.Config{TaskAuthority: "native", Host: "orca", Agent: "codex", Model: "chosen-model", ReasoningEffort: "xhigh", Sandbox: "workspace-write"}, want: []string{"Orca 本机终端", "Agent：codex", "Model：chosen-model", "Thinking effort：xhigh"}},
		{config: steerline.Config{TaskAuthority: "native", Host: "orca_supervised", Agent: "codex", Model: "remote-model", ReasoningEffort: "high", Sandbox: "workspace-write", Orca: steerline.OrcaConfig{Supervised: &steerline.SupervisedOrcaConfig{EnvironmentID: "remote-env", ConnectionID: "fixed-connection"}}}, want: []string{"Orca 受监管远程", "Model：remote-model", "Thinking effort：high", "目标环境：remote-env", "连接：fixed-connection"}},
	} {
		ref := ProjectRef{Config: sample.config}
		m := Model{width: 120, profileChoice: &executionProfileChoice{taskID: "task"}}
		shown := renderExecutionProfileChoice(m, projectState{ref: ref}, 40)
		for _, want := range sample.want {
			if !strings.Contains(shown, want) {
				t.Fatalf("execution profile missed %q: %s", want, shown)
			}
		}
	}
}
