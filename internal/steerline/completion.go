package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type CompletionRequest struct {
	SchemaVersion     int          `json:"schema_version"`
	TaskID            string       `json:"task_id"`
	RunID             string       `json:"run_id"`
	Claimant          string       `json:"claimant"`
	RepositoryPath    string       `json:"repository_path"`
	WorktreePath      string       `json:"worktree_path"`
	WorktreeBranch    string       `json:"worktree_branch"`
	BaseCommit        string       `json:"base_commit"`
	Remote            string       `json:"remote"`
	TargetBranch      string       `json:"target_branch"`
	HostingURL        string       `json:"hosting_url"`
	HostingRepository string       `json:"hosting_repository"`
	Beads             BeadsContext `json:"beads_context"`
	BD                Command      `json:"bd"`
	PR                PRInfo       `json:"pr"`
}
type CompletionReceipt struct {
	SchemaVersion int             `json:"schema_version"`
	TaskID        string          `json:"task_id"`
	RunID         string          `json:"run_id"`
	PRURL         string          `json:"pr_url"`
	Status        string          `json:"status"`
	Error         *string         `json:"error"`
	Details       json.RawMessage `json:"details"`
}

func (s *Service) complete(ctx context.Context, r *Run, pr PRInfo) error {
	if pr.State != "merged" || !pr.Merged || pr.BaseBranch != s.Config.Branch || r.PR.URL == nil || pr.URL != *r.PR.URL {
		return errors.New("completion requires freshly verified matching merge")
	}
	if r.Completion == nil {
		return s.beads.Close(ctx, r.TaskID, pr.URL)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(r.RunID) {
		return errors.New("invalid completion run ID")
	}
	if err := s.inspectRunWorkspace(ctx, r); err != nil {
		return err
	}
	if r.BeadsContext == nil {
		return errors.New("completion lacks dispatch-time Beads identity")
	}
	source, err := s.beads.Context(ctx)
	if err != nil {
		return err
	}
	if !source.Same(*r.BeadsContext) {
		return errors.New("Beads identity changed after dispatch")
	}
	task, err := s.beads.Show(ctx, r.TaskID)
	if err != nil {
		return err
	}
	if task.Assignee != r.Claimant || (task.Status != "in_progress" && task.Status != "closed") {
		return errors.New("completion task owner/status changed")
	}
	// A closed Beads task still needs the callback: a previous attempt may have
	// closed it and then failed in dependent reconciliation or viewer refresh.
	request := CompletionRequest{SchemaVersion: 1, TaskID: r.TaskID, RunID: r.RunID, Claimant: r.Claimant, RepositoryPath: s.Config.Repository, WorktreePath: *r.WorktreePath, WorktreeBranch: *r.WorktreeBranch, BaseCommit: *r.BaseCommit, Remote: s.Config.Remote, TargetBranch: s.Config.Branch, HostingURL: s.Config.PR.BaseURL, HostingRepository: s.Config.PR.Repository, Beads: source, BD: s.Config.BD, PR: pr}
	data, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Join(s.Config.RunDir, "runs", r.RunID)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err = AtomicWrite(filepath.Join(dir, "completion-request.json"), data); err != nil {
		return err
	}
	command := r.Completion.Command
	command.Args = append([]string{}, command.Args...)
	replace := strings.NewReplacer("{repository}", s.Config.Repository, "{worktree_path}", *r.WorktreePath, "{run_dir}", dir)
	for i, a := range command.Args {
		command.Args[i] = replace.Replace(a)
	}
	runner := Runner{Dir: *r.WorktreePath, Timeout: duration(r.Completion.Timeout), Input: data, Env: map[string]string{"BEADS_DIR": source.Directory, "BEADS_ACTOR": r.Claimant}}
	out, callErr := runner.Output(ctx, command)
	// Retain the adapter's structured partial-write evidence, even on exit failure.
	if err = AtomicWrite(filepath.Join(dir, "completion-result.json"), out); err != nil {
		return err
	}
	current, showErr := s.beads.Show(ctx, r.TaskID)
	if showErr == nil {
		r.BeadsStatus = current.Status
	}
	if callErr != nil {
		return callErr
	}
	if showErr != nil {
		return showErr
	}
	var receipt CompletionReceipt
	if err = decode(out, &receipt); err != nil {
		return fmt.Errorf("invalid completion receipt: %w", err)
	}
	if receipt.SchemaVersion != 1 || receipt.TaskID != r.TaskID || receipt.RunID != r.RunID || receipt.PRURL != pr.URL || receipt.Status != "completed" || receipt.Error != nil {
		return errors.New("completion receipt mismatch or incomplete operation")
	}
	if current.Status != "closed" || current.Assignee != r.Claimant {
		return errors.New("completion receipt not corroborated by Beads")
	}
	return nil
}
