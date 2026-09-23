package steerline

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"text/template"
)

//go:embed result.schema.json
var ResultSchema []byte

//go:embed prompt.tmpl
var promptTemplate string
var commitHash = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

type WorkerEvent struct {
	Session  string
	Summary  string
	Activity ActivityEvent
}
type Worker struct {
	proc   process
	Events chan WorkerEvent
	Done   chan error
}

func (w *Worker) Kill()  { w.proc.Kill() }
func (w *Worker) Close() { w.proc.Close() }
func Prompt(c Config, t Task, r *Run) (string, error) {
	contextJSON, _ := json.MarshalIndent(map[string]any{"task_id": t.ID, "run_id": r.RunID, "claimant": r.Claimant, "remote": c.Remote, "branch": c.Branch, "worktree_path": r.WorktreePath, "worktree_branch": r.WorktreeBranch, "base_commit": r.BaseCommit, "beads_directory": c.workerEnv["BEADS_DIR"], "beads_context": r.BeadsContext}, "", "  ")
	taskJSON, _ := json.MarshalIndent(t, "", "  ")
	data := map[string]string{"Context": string(contextJSON), "Task": string(taskJSON), "StartPath": c.StartSkill.Path, "FinishPath": c.FinishSkill.Path, "StartInvocation": c.StartSkill.Invocation, "FinishInvocation": c.FinishSkill.Invocation, "Workflow": c.Workflow}
	tmpl, e := template.New("prompt").Parse(promptTemplate)
	if e != nil {
		return "", e
	}
	var b bytes.Buffer
	e = tmpl.Execute(&b, data)
	return b.String(), e
}

// StartWorker prepares the per-run files and hands the actual process launch to
// the configured host. Scheduling, recovery and result authority stay here.
func StartWorker(c Config, t Task, r *Run) (*Worker, error) {
	if r.WorktreePath == nil || r.BaseCommit == nil || r.WorktreeBranch == nil || canonical(c.Repository) != canonical(*r.WorktreePath) || c.workerEnv["BEADS_DIR"] == "" || c.workerEnv["BEADS_ACTOR"] != r.Claimant {
		return nil, errors.New("worker requires verified workspace and shared Beads environment")
	}
	if r.ResultPath == nil || r.LogPath == nil {
		return nil, errors.New("worker output paths are missing")
	}
	dir := filepath.Dir(*r.ResultPath)
	if !inside(filepath.Join(c.RunDir, "runs", r.RunID), dir) || filepath.Base(*r.ResultPath) != "result.json" || filepath.Base(*r.LogPath) != "events.jsonl" || canonical(filepath.Dir(*r.LogPath)) != canonical(dir) {
		return nil, errors.New("worker output paths must share a directory inside this run")
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	schema := filepath.Join(dir, "result.schema.json")
	if e := AtomicWrite(schema, ResultSchema); e != nil {
		return nil, e
	}
	prompt, e := Prompt(c, t, r)
	if e != nil {
		return nil, e
	}
	if e = AtomicWrite(filepath.Join(dir, "prompt.txt"), []byte(prompt)); e != nil {
		return nil, e
	}
	args := append([]string{}, c.Codex.Args...)
	args = append(args, "exec", "--json", "--output-schema", schema, "--output-last-message", *r.ResultPath, "--sandbox", c.Sandbox)
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	if c.Profile != "" {
		args = append(args, "--profile", c.Profile)
	}
	args = append(args, "-")
	spec := SpawnSpec{
		Dir:        c.Repository,
		EventsPath: *r.LogPath,
		StderrPath: filepath.Join(dir, "stderr.log"),
		Command:    Command{Executable: c.Codex.Executable, Args: args},
		Stdin:      bytes.NewReader([]byte(prompt)),
		Env:        c.workerEnv,
	}
	return SpawnerFor(c).Start(context.Background(), spec)
}

func ReadResult(path string, r *Run) (Result, error) {
	f, e := os.Open(path)
	if e != nil {
		return Result{}, e
	}
	defer f.Close()
	stat, e := f.Stat()
	if e != nil {
		return Result{}, e
	}
	if stat.Size() > 2<<20 {
		return Result{}, errors.New("result exceeds 2 MiB")
	}
	b, e := io.ReadAll(io.LimitReader(f, 2<<20))
	if e != nil {
		return Result{}, e
	}
	var fields map[string]json.RawMessage
	if e = json.Unmarshal(b, &fields); e != nil {
		return Result{}, e
	}
	for _, name := range []string{"task_id", "run_id", "outcome", "pr_url", "worktree_path", "base_commit", "summary", "error"} {
		if _, ok := fields[name]; !ok {
			return Result{}, fmt.Errorf("missing result field %s", name)
		}
	}
	var result Result
	if e = decode(b, &result); e != nil {
		return result, e
	}
	if result.TaskID != r.TaskID || result.RunID != r.RunID {
		return result, errors.New("result task_id/run_id mismatch")
	}
	switch result.Outcome {
	case "pr_submitted":
		if result.PRURL == nil || *result.PRURL == "" || result.WorktreePath == nil || !filepath.IsAbs(*result.WorktreePath) || result.BaseCommit == nil || !commitHash.MatchString(*result.BaseCommit) {
			return result, errors.New("submitted PR requires URL, absolute worktree_path and full base_commit")
		}
	case "blocked":
	case "needs_decision":
		if e := validateDecisionRequest(result.Decision); e != nil {
			return result, e
		}
	default:
		return result, errors.New("invalid result outcome")
	}
	return result, nil
}
