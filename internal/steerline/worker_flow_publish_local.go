package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// The final line is an explicit handoff from the sandboxed worker. Only the
// center may use its GitHub credentials to publish the fixed execution branch.
func localWorkerPublishInstruction(runID string) string {
	return "\nLocal publication: stage only the files you intend to deliver. Do not commit, push, or create a PR from this Codex session. When the task is ready for a draft PR, end your final reply with this exact line (outside a code block):\nSTEERLINE_PUBLISH_READY " + runID + "\nIf you still need input or the work is incomplete, omit that line.\n"
}

func localWorkerRequestedPublish(message, runID string) bool {
	if !stableExecutionID(runID) {
		return false
	}
	lines := strings.Split(strings.TrimSpace(message), "\n")
	return len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "STEERLINE_PUBLISH_READY "+runID
}

func (s *Service) localWorkerRun(runID string) *Run {
	for _, run := range s.State.Runs {
		if run != nil && run.RunID == runID {
			return run
		}
	}
	return nil
}

// publishLocalWorkerRun executes Git and GitHub operations in the service's
// host identity. The worker supplies only staged content and an exact run-ID
// handoff; it never receives the host's credentials.
func (s *Service) publishLocalWorkerRun(ctx context.Context, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, turnID string) (PRInfo, error) {
	var info PRInfo
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return info, configErr
	}
	c := fixed.config
	if run == nil || workflow == nil || execution == nil || run.WorkspaceIntent == nil || run.WorktreePath == nil ||
		workflow.WorkflowRunID != run.RunID || execution.WorkflowRunID != run.RunID || run.Protocol != string(ProtocolLocalCodexAppServer) || !codexThreadID.MatchString(turnID) ||
		execution.CurrentTurnID != turnID || execution.LastStoppedTurnID != turnID ||
		c.hostName() != "local" || c.PR.Kind != "command" ||
		strings.TrimSuffix(c.PR.BaseURL, "/") != "https://github.com" || c.PR.Query.Executable == "" {
		return info, errors.New("local host publication requires a fixed GitHub command provider and worker execution")
	}
	if canonical(*run.WorktreePath) != canonical(run.WorkspaceIntent.Path) || run.WorktreeBranch == nil ||
		*run.WorktreeBranch != run.WorkspaceIntent.Branch || execution.Workspace.Branch != *run.WorktreeBranch ||
		workflow.Branch == "" || c.PR.Repository == "" {
		return info, errors.New("local host publication workspace or target branch differs from the fixed execution")
	}
	git := gitRunner(c, *run.WorktreePath)
	gitText := func(args ...string) (string, error) {
		out, err := git.Output(ctx, c.Git, args...)
		return strings.TrimSpace(string(out)), err
	}
	common, err := gitText("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || canonical(common) != canonical(c.identity) {
		return info, errors.Join(err, errors.New("local publication Git common directory changed"))
	}
	branch, err := gitText("symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || branch != *run.WorktreeBranch {
		return info, errors.Join(err, errors.New("local publication branch changed"))
	}
	startingHead, err := gitText("rev-parse", "HEAD")
	if err != nil || !validCommit(startingHead) {
		return info, errors.Join(err, errors.New("local publication HEAD is unavailable"))
	}
	if _, err = gitText("merge-base", "--is-ancestor", run.WorkspaceIntent.BaseCommit, "HEAD"); err != nil {
		return info, fmt.Errorf("local publication branch no longer descends from its fixed base: %w", err)
	}
	unstaged, err := gitText("diff", "--name-only")
	if err != nil || unstaged != "" {
		return info, errors.Join(err, errors.New("local publication has unstaged changes"))
	}
	untracked, err := gitText("ls-files", "--others", "--exclude-standard")
	if err != nil || untracked != "" {
		return info, errors.Join(err, errors.New("local publication has untracked files"))
	}
	staged, err := gitText("diff", "--cached", "--name-only")
	if err != nil {
		return info, err
	}
	if staged == "" && startingHead == run.WorkspaceIntent.BaseCommit {
		return info, errors.New("local publication has no staged changes or candidate commit")
	}
	indexTree, err := gitText("write-tree")
	if err != nil || !validCommit(indexTree) {
		return info, errors.Join(err, errors.New("local publication index tree is unavailable"))
	}
	opID, err := s.startOperation("publish_local_worker", "local-publish:"+run.RunID+":"+turnID, struct {
		RunID        string `json:"run_id"`
		TurnID       string `json:"turn_id"`
		Branch       string `json:"branch"`
		StartingHead string `json:"starting_head"`
		IndexTree    string `json:"index_tree"`
	}{run.RunID, turnID, branch, startingHead, indexTree})
	if err != nil {
		return info, err // An earlier uncertain publication must be reconciled, not repeated.
	}
	run = s.localWorkerRun(run.RunID)
	workflow, execution = s.workflowRecords(run)
	if run == nil || workflow == nil || execution == nil {
		return info, errors.New("fixed worker execution disappeared after publication intent was saved")
	}
	confirmed := false
	defer func() {
		if !confirmed {
			_ = s.changeOperation(opID, "outcome_unknown", "", nil, "local GitHub publication requires reconciliation")
		}
	}()
	if staged != "" {
		if _, err = gitText("commit", "-m", "SteerLine task "+run.TaskID); err != nil {
			return info, fmt.Errorf("host Git commit failed: %w", err)
		}
	}
	head, err := gitText("rev-parse", "HEAD")
	if err != nil || !validCommit(head) {
		return info, errors.Join(err, errors.New("host commit head is unavailable"))
	}
	if _, err = gitText("push", c.Remote, "HEAD:refs/heads/"+branch); err != nil {
		return info, fmt.Errorf("host Git push failed: %w", err)
	}
	url, err := s.localWorkerPRURL(ctx, run, workflow, head)
	if err != nil {
		return info, err
	}
	info, err = s.prs.Query(ctx, url)
	if err != nil || info.HeadRevision != head {
		return PRInfo{}, errors.Join(err, errors.New("published PR head does not match the fixed local commit"))
	}
	if err = BindWorkerPR(workflow, execution, info, url, c.PR.Repository, false); err != nil {
		return PRInfo{}, err
	}
	s.refreshRun(run)
	if err = s.changeOperation(opID, "confirmed", url, mustJSON(info), ""); err != nil {
		return PRInfo{}, err
	}
	confirmed = true
	return info, nil
}

func (s *Service) localWorkerPRURL(ctx context.Context, run *Run, workflow *WorkflowRunRecord, head string) (string, error) {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return "", configErr
	}
	c := fixed.config
	existing, err := s.findLocalWorkerPRURL(ctx, run, workflow, head)
	if err != nil || existing != "" {
		return existing, err
	}
	repo := c.PR.Repository
	gh := Command{Executable: c.PR.Query.Executable}
	runner := Runner{Dir: *run.WorktreePath, Timeout: duration(c.CommandTimeout)}
	created, err := runner.Output(ctx, gh, "pr", "create", "--repo", repo, "--base", workflow.Branch,
		"--head", *run.WorktreeBranch, "--draft", "--title", "SteerLine task "+run.TaskID,
		"--body", "SteerLine worker-flow run "+run.RunID)
	if err != nil {
		return "", fmt.Errorf("create draft PR from fixed branch: %w", err)
	}
	url := strings.TrimSpace(string(created))
	if _, err = c.PR.PRNumber(url); err != nil {
		return "", fmt.Errorf("created PR URL is invalid: %w", err)
	}
	return url, nil
}

func (s *Service) findLocalWorkerPRURL(ctx context.Context, run *Run, workflow *WorkflowRunRecord, head string) (string, error) {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return "", configErr
	}
	c := fixed.config
	repo := c.PR.Repository
	owner, _, ok := strings.Cut(repo, "/")
	if !ok || owner == "" {
		return "", errors.New("local GitHub publication repository is invalid")
	}
	gh := Command{Executable: c.PR.Query.Executable}
	runner := Runner{Dir: *run.WorktreePath, Timeout: duration(c.CommandTimeout)}
	endpoint := fmt.Sprintf("repos/%s/pulls?state=all&head=%s:%s&per_page=100", repo, owner, *run.WorktreeBranch)
	out, err := runner.Output(ctx, gh, "api", endpoint)
	if err != nil {
		return "", fmt.Errorf("query exact branch PR before publication: %w", err)
	}
	var prs []struct {
		URL   string `json:"html_url"`
		State string `json:"state"`
		Head  struct {
			Ref  string `json:"ref"`
			SHA  string `json:"sha"`
			Repo struct {
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	}
	if err = json.Unmarshal(out, &prs); err != nil {
		return "", fmt.Errorf("decode exact branch PR query: %w", err)
	}
	var existing string
	for _, pr := range prs {
		if pr.Head.Ref != *run.WorktreeBranch || pr.Head.Repo.FullName != repo || pr.Base.Ref != workflow.Branch {
			continue
		}
		if pr.Head.SHA != head || pr.State != "open" || existing != "" {
			return "", errors.New("fixed branch already has an incompatible or ambiguous PR")
		}
		existing = pr.URL
	}
	if existing != "" {
		return existing, nil
	}
	return "", nil
}

// A service exit can happen after the host has pushed or created the PR but
// before it has saved the receipt. Reconcile only observed, exact identities;
// never issue another create from an uncertain publication operation.
func (s *Service) reconcileLocalWorkerPublication(ctx context.Context, run *Run) (bool, error) {
	if run == nil {
		return false, nil
	}
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return false, configErr
	}
	c := fixed.config
	if c.hostName() != "local" {
		return false, nil
	}
	var op *OperationRecord
	for i := range s.State.Operations {
		candidate := &s.State.Operations[i]
		if candidate.Kind == "publish_local_worker" && strings.HasPrefix(candidate.IdempotencyKey, "local-publish:"+run.RunID+":") &&
			(candidate.Status == "in_flight" || candidate.Status == "outcome_unknown") {
			if op != nil {
				return false, errors.New("several local publication operations need manual reconciliation")
			}
			op = candidate
		}
	}
	if op == nil {
		return false, nil
	}
	var intent struct {
		RunID        string `json:"run_id"`
		TurnID       string `json:"turn_id"`
		Branch       string `json:"branch"`
		StartingHead string `json:"starting_head"`
		IndexTree    string `json:"index_tree"`
	}
	if err := json.Unmarshal(op.Request, &intent); err != nil || intent.RunID != run.RunID || run.Protocol != string(ProtocolLocalCodexAppServer) || !codexThreadID.MatchString(intent.TurnID) ||
		run.WorktreePath == nil || run.WorktreeBranch == nil || intent.Branch != *run.WorktreeBranch ||
		!validCommit(intent.StartingHead) || !validCommit(intent.IndexTree) {
		return false, errors.Join(err, errors.New("saved local publication intent is invalid"))
	}
	workflow, execution := s.workflowRecords(run)
	if workflow == nil || execution == nil || execution.LastStoppedTurnID != intent.TurnID ||
		workflow.Phase != "waiting_decision" || run.FinishTaskStatus != "waiting_decision" {
		return false, errors.New("saved local publication no longer matches the stopped worker turn")
	}
	git := gitRunner(c, *run.WorktreePath)
	gitText := func(args ...string) (string, error) {
		out, err := git.Output(ctx, c.Git, args...)
		return strings.TrimSpace(string(out)), err
	}
	common, err := gitText("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || canonical(common) != canonical(c.identity) {
		return false, errors.Join(err, errors.New("recovery Git common directory changed"))
	}
	branch, err := gitText("symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || branch != intent.Branch {
		return false, errors.Join(err, errors.New("recovery branch changed"))
	}
	head, err := gitText("rev-parse", "HEAD")
	if err != nil || !validCommit(head) {
		return false, errors.Join(err, errors.New("recovery commit is unavailable"))
	}
	tree, err := gitText("rev-parse", "HEAD^{tree}")
	if err != nil || tree != intent.IndexTree {
		return false, errors.Join(err, errors.New("recovery commit tree differs from the saved staged content"))
	}
	if _, err = gitText("merge-base", "--is-ancestor", intent.StartingHead, "HEAD"); err != nil {
		return false, fmt.Errorf("recovery commit changed ancestry: %w", err)
	}
	remote, err := gitText("ls-remote", c.Remote, "refs/heads/"+intent.Branch)
	if err != nil || !strings.HasPrefix(remote, head+"\trefs/heads/"+intent.Branch) {
		return false, errors.Join(err, errors.New("fixed remote branch does not match the local candidate"))
	}
	url, err := s.findLocalWorkerPRURL(ctx, run, workflow, head)
	if err != nil || url == "" {
		return false, errors.Join(err, errors.New("fixed remote branch has no verified PR; publication remains uncertain"))
	}
	info, err := s.prs.Query(ctx, url)
	if err != nil || info.HeadRevision != head {
		return false, errors.Join(err, errors.New("recovered PR head does not match the fixed commit"))
	}
	if err = BindWorkerPR(workflow, execution, info, url, c.PR.Repository, false); err != nil {
		return false, err
	}
	s.refreshRun(run)
	if err = s.offerWorkerPRFeedback(run, url); err != nil {
		return false, err
	}
	if err = s.setOperationState(op.OperationID, "confirmed", url, mustJSON(info), ""); err != nil {
		return false, err
	}
	if err = s.save(); err != nil {
		return false, err
	}
	return true, nil
}
