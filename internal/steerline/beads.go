package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"
)

type Task struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Status      string         `json:"status"`
	Type        string         `json:"issue_type"`
	Assignee    string         `json:"assignee"`
	Priority    *int           `json:"priority"`
	Description string         `json:"description"`
	SpecID      string         `json:"spec_id"`
	Metadata    map[string]any `json:"metadata"`
	Labels      []string       `json:"labels"`
}

// TaskDetail is the read-only, human-facing view of one provider task. It is a
// viewer convenience: it never claims, closes or otherwise mutates state.
type TaskDetail struct {
	ID          string
	Title       string
	Type        string
	Status      string
	Assignee    string
	Priority    *int
	Description string
	SpecID      string
	Labels      []string
}

// LoadTaskDetail uses the configured task authority for the viewer's detail pane.
func LoadTaskDetail(ctx context.Context, c Config, id string) (TaskDetail, error) {
	if c.taskAuthority() == "native" {
		if err := ValidateNativeTaskID(id); err != nil {
			return TaskDetail{}, err
		}
		store, err := OpenNativeTaskProject(ctx, &c)
		if err != nil {
			return TaskDetail{}, err
		}
		defer store.Close()
		page, err := (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, id)
		if err != nil {
			return TaskDetail{}, err
		}
		task := page.Tasks[0]
		return TaskDetail{ID: task.TaskID, Title: task.Title, Type: task.Type, Status: task.Status, Assignee: task.Claimant, Priority: &task.Priority, SpecID: task.Contract.SpecRef.Path, Description: fmt.Sprintf("Spec: %s @ %s\nPlan: %s @ %s\nSampled: %s", task.Contract.SpecRef.Path, task.Contract.SpecRef.Commit, task.Contract.PlanRef.Path, task.Contract.PlanRef.Commit, page.SampledAt.Format(time.RFC3339Nano))}, nil
	}
	r := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	b := Beads{Runner: r, Command: c.BD, Claimant: c.Claimant}
	t, e := b.Show(ctx, id)
	if e != nil {
		return TaskDetail{}, e
	}
	return TaskDetail{ID: t.ID, Title: t.Title, Type: t.Type, Status: t.Status, Assignee: t.Assignee, Priority: t.Priority, Description: t.Description, SpecID: t.SpecID, Labels: t.Labels}, nil
}

type BeadsContext struct {
	Directory string `json:"beads_dir"`
	ProjectID string `json:"project_id"`
	Database  string `json:"database"`
	Backend   string `json:"backend"`
}

func (b Beads) Context(ctx context.Context) (BeadsContext, error) {
	var c BeadsContext
	out, e := b.Runner.Output(ctx, b.Command, "context", "--json")
	if e != nil {
		return c, e
	}
	if e = json.Unmarshal(out, &c); e != nil {
		return c, e
	}
	if !filepath.IsAbs(c.Directory) || c.ProjectID == "" || c.Database == "" || c.Backend == "" {
		return c, errors.New("Beads context requires absolute beads_dir, project_id, database and backend")
	}
	c.Directory, e = filepath.EvalSymlinks(c.Directory)
	return c, e
}
func (c BeadsContext) Same(other BeadsContext) bool {
	return canonical(c.Directory) == canonical(other.Directory) && c.ProjectID == other.ProjectID && c.Database == other.Database && c.Backend == other.Backend
}

type Beads struct {
	Runner   Runner
	Command  Command
	Claimant string
}

func parseTasks(b []byte) ([]Task, error) {
	var t []Task
	if e := json.Unmarshal(b, &t); e != nil {
		return nil, fmt.Errorf("invalid Beads JSON: %w", e)
	}
	if t == nil {
		return nil, errors.New("Beads must return an array, not null")
	}
	for _, v := range t {
		if v.ID == "" || v.Status == "" || v.Type == "" {
			return nil, errors.New("Beads task missing id/status/issue_type")
		}
	}
	return t, nil
}
func (b Beads) list(ctx context.Context, args ...string) ([]Task, error) {
	out, e := b.Runner.Output(ctx, b.Command, append(args, "--json")...)
	if e != nil {
		return nil, e
	}
	return parseTasks(out)
}
func (b Beads) Ready(ctx context.Context) ([]Task, error) {
	return b.list(ctx, "ready", "--unassigned", "--limit", "0")
}
func (b Beads) Owned(ctx context.Context) ([]Task, error) {
	return b.list(ctx, "list", "--assignee", b.Claimant, "--all", "--limit", "0")
}
func (b Beads) Show(ctx context.Context, id string) (Task, error) {
	t, e := b.list(ctx, "show", "--id", id)
	if e != nil {
		return Task{}, e
	}
	if len(t) != 1 || t[0].ID != id {
		return Task{}, errors.New("Beads show returned mismatched task")
	}
	return t[0], nil
}
func (b Beads) Claim(ctx context.Context, id string) (bool, error) {
	// bd 1.1.2 can exit zero with no JSON on a claim conflict. Never trust exit code alone.
	out, cmdErr := b.Runner.Output(ctx, b.Command, "update", id, "--claim", "--actor", b.Claimant, "--json")
	t, showErr := b.Show(ctx, id)
	if showErr != nil {
		return false, showErr
	}
	if t.Assignee != "" && t.Assignee != b.Claimant {
		return false, nil
	}
	if cmdErr != nil {
		return false, fmt.Errorf("claim outcome uncertain: %w", cmdErr)
	}
	claimed, e := parseTasks(out)
	if e != nil {
		return false, e
	}
	if len(claimed) != 1 || claimed[0].ID != id || claimed[0].Assignee != b.Claimant || claimed[0].Status != "in_progress" || t.Assignee != b.Claimant || t.Status != "in_progress" {
		return false, errors.New("claim not authoritatively confirmed")
	}
	return true, nil
}
func (b Beads) Close(ctx context.Context, id, url string) error {
	t, e := b.Show(ctx, id)
	if e != nil {
		return e
	}
	if t.Status == "closed" {
		return nil
	}
	if t.Assignee != b.Claimant {
		return errors.New("task ownership changed; refusing close")
	}
	_, e = b.Runner.Output(ctx, b.Command, "close", id, "--reason", "Verified merged PR: "+url, "--actor", b.Claimant, "--json")
	if e != nil {
		return e
	}
	t, e = b.Show(ctx, id)
	if e != nil {
		return e
	}
	if t.Status != "closed" {
		return errors.New("Beads close did not persist closed status")
	}
	return nil
}
func executable(t Task, types []string) bool {
	if t.Status != "open" || t.Assignee != "" {
		return false
	}
	if t.Metadata["split.role"] == "aggregate" {
		return false
	}
	for _, label := range t.Labels {
		if label == "split-aggregate" {
			return false
		}
	}
	for _, typ := range types {
		if t.Type == typ {
			return true
		}
	}
	return false
}
