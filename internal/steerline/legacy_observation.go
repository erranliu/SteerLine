package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ErrNotLegacyBeadsConfig distinguishes an unrelated bad config from the two
// retired Beads projects that the console may observe during their cutover.
var ErrNotLegacyBeadsConfig = errors.New("not a Beads project config")

// LegacyBeadsProject is a read-only source. It is never passed to a native
// command, service constructor, or task admission path.
type LegacyBeadsProject struct {
	Repository  string
	RunDir      string
	RuntimeFile string
	BD          Command
}

type LegacyBeadsTask struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Status       string   `json:"status"`
	Type         string   `json:"issue_type"`
	Priority     int      `json:"priority"`
	Assignee     string   `json:"assignee"`
	Predecessors []string `json:"-"`
}

type LegacyBeadsRun struct {
	TaskID          string
	ExecutionStatus string
	FinishStatus    string
	PRURL           string
	PRState         string
	Error           string
}

type LegacyBeadsSnapshot struct {
	Tasks           []LegacyBeadsTask
	Runs            []LegacyBeadsRun
	SchemaVersion   int
	ServiceStatus   string
	ServicePID      int
	ServiceLiveness string
	HeartbeatAt     time.Time
	SampledAt       time.Time
	RuntimeError    string
	DependencyError string
}

// LoadLegacyBeadsProject recognizes only an explicit old Beads configuration
// backed by a Beads repository. It does not initialize or migrate either state.
func LoadLegacyBeadsProject(path string) (*LegacyBeadsProject, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if fields["bd"] == nil {
		return nil, ErrNotLegacyBeadsConfig
	}
	var raw struct {
		Repository    string  `json:"repository"`
		RunDir        string  `json:"run_dir"`
		RuntimeFile   string  `json:"runtime_file"`
		TaskAuthority string  `json:"task_authority"`
		Git           Command `json:"git"`
		BD            Command `json:"bd"`
	}
	if err = json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw.TaskAuthority != "" && raw.TaskAuthority != "beads" {
		return nil, errors.New("Beads view requires the retired Beads task authority")
	}
	if raw.Repository == "" || raw.BD.Executable == "" {
		return nil, errors.New("Beads view requires a repository and bd executable")
	}
	if name := strings.ToLower(filepath.Base(raw.BD.Executable)); name != "bd" && name != "bd.exe" || len(raw.BD.Args) != 0 {
		return nil, errors.New("Beads view requires an unmodified bd executable command")
	}
	if !filepath.IsAbs(raw.Repository) {
		raw.Repository = filepath.Join(filepath.Dir(path), raw.Repository)
	}
	if info, statErr := os.Stat(filepath.Join(raw.Repository, ".beads")); statErr != nil || !info.IsDir() {
		return nil, errors.New("configured repository has no Beads authority directory")
	}
	if raw.RuntimeFile == "" {
		raw.RuntimeFile = "runtime.json"
	}
	if filepath.Base(raw.RuntimeFile) != raw.RuntimeFile || raw.RuntimeFile == "." {
		return nil, errors.New("legacy runtime_file must be a filename")
	}
	if raw.RunDir == "" {
		if raw.Git.Executable == "" {
			raw.Git.Executable = "git"
		}
		key, keyErr := repoStateKey(Config{Repository: raw.Repository, Git: raw.Git, CommandTimeout: "30s"})
		if keyErr != nil {
			return nil, fmt.Errorf("derive Beads runtime location: %w", keyErr)
		}
		raw.RunDir = filepath.Join(SteerlineStateRoot(), key)
	} else if !filepath.IsAbs(raw.RunDir) {
		raw.RunDir = filepath.Join(filepath.Dir(path), raw.RunDir)
	}
	root, err := filepath.Abs(SteerlineStateRoot())
	if err != nil {
		return nil, err
	}
	runDir, err := filepath.Abs(raw.RunDir)
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(root, runDir)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, errors.New("legacy runtime is outside the SteerLine state root")
	}
	return &LegacyBeadsProject{Repository: raw.Repository, RunDir: runDir, RuntimeFile: raw.RuntimeFile, BD: raw.BD}, nil
}

// Read observes Beads through its read-only CLI and the old runtime file. The
// Beads query is the task authority; the runtime is only a sampled execution view.
func (p LegacyBeadsProject) Read(ctx context.Context) (LegacyBeadsSnapshot, error) {
	snapshot := LegacyBeadsSnapshot{SampledAt: time.Now().UTC(), Tasks: []LegacyBeadsTask{}, Runs: []LegacyBeadsRun{}}
	p.readRuntime(&snapshot)
	readCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	data, err := p.bdOutput(readCtx, "list", "--all", "--limit", "0", "--json")
	if err != nil {
		return snapshot, fmt.Errorf("read Beads tasks: %w", err)
	}
	if err = json.Unmarshal(data, &snapshot.Tasks); err != nil {
		return snapshot, fmt.Errorf("decode Beads tasks: %w", err)
	}
	if len(snapshot.Tasks) == 0 {
		return snapshot, nil
	}
	ids := make([]string, 0, len(snapshot.Tasks))
	byID := make(map[string]*LegacyBeadsTask, len(snapshot.Tasks))
	for i := range snapshot.Tasks {
		ids = append(ids, snapshot.Tasks[i].ID)
		byID[snapshot.Tasks[i].ID] = &snapshot.Tasks[i]
	}
	args := append([]string{"dep", "list"}, ids...)
	args = append(args, "--json")
	dependencies, depErr := p.bdOutput(readCtx, args...)
	if depErr != nil {
		snapshot.DependencyError = depErr.Error()
		return snapshot, nil
	}
	var edges []struct {
		IssueID     string `json:"issue_id"`
		DependsOnID string `json:"depends_on_id"`
		Type        string `json:"type"`
	}
	if err = json.Unmarshal(dependencies, &edges); err != nil {
		snapshot.DependencyError = err.Error()
		return snapshot, nil
	}
	for _, edge := range edges {
		if edge.Type == "blocks" && byID[edge.IssueID] != nil {
			byID[edge.IssueID].Predecessors = append(byID[edge.IssueID].Predecessors, edge.DependsOnID)
		}
	}
	return snapshot, nil
}

func (p LegacyBeadsProject) bdOutput(ctx context.Context, args ...string) ([]byte, error) {
	commandArgs := []string{"--readonly", "-C", p.Repository}
	commandArgs = append(commandArgs, args...)
	command := exec.CommandContext(ctx, p.BD.Executable, commandArgs...)
	return command.Output()
}

func (p LegacyBeadsProject) readRuntime(snapshot *LegacyBeadsSnapshot) {
	// This projection is diagnostic. Unknown or unsupported old formats never
	// become a current service connection or a native start permission.
	data, err := readSnapshot(filepath.Join(p.RunDir, p.RuntimeFile))
	if err != nil {
		snapshot.RuntimeError = err.Error()
		return
	}
	if len(data) > 4<<20 {
		snapshot.RuntimeError = "legacy runtime projection exceeds 4 MiB"
		return
	}
	var runtimeState struct {
		SchemaVersion int       `json:"schema_version"`
		HeartbeatAt   time.Time `json:"heartbeat_at"`
		Service       struct {
			Status          string           `json:"status"`
			PID             int              `json:"pid"`
			ProcessIdentity *ProcessIdentity `json:"process_identity"`
		} `json:"service"`
		Runs []struct {
			TaskID          string `json:"task_id"`
			ExecutionStatus string `json:"execution_status"`
			FinishStatus    string `json:"finish_task_status"`
			Error           string `json:"error"`
			PR              struct {
				URL   string `json:"url"`
				State string `json:"state"`
			} `json:"pr"`
		} `json:"runs"`
	}
	if err = json.Unmarshal(data, &runtimeState); err != nil {
		snapshot.RuntimeError = err.Error()
		return
	}
	if runtimeState.SchemaVersion != 3 && runtimeState.SchemaVersion != 4 {
		snapshot.RuntimeError = fmt.Sprintf("unsupported legacy runtime schema %d", runtimeState.SchemaVersion)
		return
	}
	snapshot.SchemaVersion = runtimeState.SchemaVersion
	snapshot.HeartbeatAt = runtimeState.HeartbeatAt
	snapshot.ServiceStatus = runtimeState.Service.Status
	snapshot.ServicePID = runtimeState.Service.PID
	snapshot.ServiceLiveness = "unknown"
	if runtimeState.Service.ProcessIdentity != nil {
		switch inspectProcess(runtimeState.Service.ProcessIdentity) {
		case "matching process still exists":
			snapshot.ServiceLiveness = "live"
		case "process absent":
			snapshot.ServiceLiveness = "exited"
		}
	}
	for _, run := range runtimeState.Runs {
		snapshot.Runs = append(snapshot.Runs, LegacyBeadsRun{TaskID: run.TaskID, ExecutionStatus: run.ExecutionStatus, FinishStatus: run.FinishStatus, PRURL: run.PR.URL, PRState: run.PR.State, Error: run.Error})
	}
}
