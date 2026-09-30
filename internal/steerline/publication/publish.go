package publication

import (
	"context"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"strings"
)

type GitRunner interface {
	Output(context.Context, contracts.Command, ...string) ([]byte, error)
}
type HostPublisher struct {
	Git        GitRunner
	GitCommand contracts.Command
	PR         *Client
}
type Stage string

const (
	StageCandidate   Stage = "candidate"
	StageCommitted   Stage = "committed"
	StagePushed      Stage = "pushed"
	StagePullRequest Stage = "pull_request"
)

type Result struct {
	Request Request           `json:"request"`
	Stage   Stage             `json:"stage"`
	Head    string            `json:"head,omitempty"`
	PRURL   string            `json:"pr_url,omitempty"`
	PR      *contracts.PRInfo `json:"pr,omitempty"`
}

func (r Result) Complete() bool {
	return r.Stage == StagePullRequest && r.Head != "" && r.PRURL != "" && r.PR != nil
}
func (p *HostPublisher) git(ctx context.Context, args ...string) (string, error) {
	out, err := p.Git.Output(ctx, p.GitCommand, args...)
	if err != nil && p.PR != nil {
		err = p.PR.redact(err)
	}
	return strings.TrimSpace(string(out)), err
}
func (p *HostPublisher) Preflight(ctx context.Context, request Request) (Request, error) {
	if err := request.Validate(); err != nil {
		return Request{}, err
	}
	if p.Git == nil || p.PR == nil {
		return Request{}, errors.New("local publisher is not configured")
	}
	common, err := p.git(ctx, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || canonical(common) != canonical(request.GitCommonDir) {
		return Request{}, errors.Join(err, errors.New("local publication Git common directory changed"))
	}
	branch, err := p.git(ctx, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || branch != request.Workspace.Branch {
		return Request{}, errors.Join(err, errors.New("local publication branch changed"))
	}
	head, err := p.git(ctx, "rev-parse", "HEAD")
	if err != nil || !validCommit(head) {
		return Request{}, errors.Join(err, errors.New("local publication HEAD is unavailable"))
	}
	if _, err = p.git(ctx, "merge-base", "--is-ancestor", request.Workspace.BaseCommit, "HEAD"); err != nil {
		return Request{}, fmt.Errorf("local publication branch no longer descends from its fixed base: %w", err)
	}
	unstaged, err := p.git(ctx, "diff", "--name-only")
	if err != nil || unstaged != "" {
		return Request{}, errors.Join(err, errors.New("local publication has unstaged changes"))
	}
	untracked, err := p.git(ctx, "ls-files", "--others", "--exclude-standard")
	if err != nil || untracked != "" {
		return Request{}, errors.Join(err, errors.New("local publication has untracked files"))
	}
	staged, err := p.git(ctx, "diff", "--cached", "--name-only")
	if err != nil {
		return Request{}, err
	}
	if staged == "" && head == request.Workspace.BaseCommit {
		return Request{}, errors.New("local publication has no staged changes or candidate commit")
	}
	tree, err := p.git(ctx, "write-tree")
	if err != nil || !validCommit(tree) {
		return Request{}, errors.Join(err, errors.New("local publication index tree is unavailable"))
	}
	request.StartingHead, request.IndexTree = head, tree
	request.Digest = requestDigest(request)
	return request, nil
}
func (p *HostPublisher) Publish(ctx context.Context, request Request, record func(Result) error) (result Result, err error) {
	result = Result{Request: request, Stage: StageCandidate}
	if err = request.ValidateCandidate(); err != nil {
		return result, err
	}
	if p.Git == nil || p.PR == nil {
		return result, errors.New("local publisher is not configured")
	}
	defer func() { err = p.PR.redact(err) }()
	common, e := p.git(ctx, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if e != nil || canonical(common) != canonical(request.GitCommonDir) {
		return result, errors.Join(e, errors.New("local publication Git common directory changed"))
	}
	branch, e := p.git(ctx, "symbolic-ref", "--quiet", "--short", "HEAD")
	if e != nil || branch != request.Workspace.Branch {
		return result, errors.Join(e, errors.New("local publication branch changed"))
	}
	head, e := p.git(ctx, "rev-parse", "HEAD")
	if e != nil || head != request.StartingHead {
		return result, errors.Join(e, errors.New("local publication HEAD changed after intent was saved"))
	}
	tree, e := p.git(ctx, "write-tree")
	if e != nil || tree != request.IndexTree {
		return result, errors.Join(e, errors.New("local publication index differs from its saved candidate"))
	}
	unstaged, e := p.git(ctx, "diff", "--name-only")
	if e != nil || unstaged != "" {
		return result, errors.Join(e, errors.New("local publication has unstaged changes"))
	}
	untracked, e := p.git(ctx, "ls-files", "--others", "--exclude-standard")
	if e != nil || untracked != "" {
		return result, errors.Join(e, errors.New("local publication has untracked files"))
	}
	staged, e := p.git(ctx, "diff", "--cached", "--name-only")
	if e != nil {
		return result, e
	}
	if staged != "" {
		if _, e = p.git(ctx, "commit", "-m", "SteerLine task "+request.TaskID); e != nil {
			return result, fmt.Errorf("host Git commit failed: %w", e)
		}
	}
	head, e = p.git(ctx, "rev-parse", "HEAD")
	if e != nil || !validCommit(head) {
		return result, errors.Join(e, errors.New("host commit head is unavailable"))
	}
	result.Head, result.Stage = head, StageCommitted
	if record != nil {
		if err = record(result); err != nil {
			return result, err
		}
	}
	if _, e = p.git(ctx, "push", request.Remote, "HEAD:refs/heads/"+request.Workspace.Branch); e != nil {
		return result, fmt.Errorf("host Git push failed: %w", e)
	}
	result.Stage = StagePushed
	if record != nil {
		if err = record(result); err != nil {
			return result, err
		}
	}
	url, e := p.PR.Create(ctx, request, head)
	if e != nil {
		return result, e
	}
	info, e := p.PR.Query(ctx, url)
	if e != nil || info.HeadRevision != head {
		return result, errors.Join(e, errors.New("published PR head does not match the fixed local commit"))
	}
	result.Stage, result.PRURL, result.PR = StagePullRequest, url, &info
	if record != nil {
		if err = record(result); err != nil {
			return result, err
		}
	}
	return result, nil
}

// Reconcile only queries the saved candidate and its exact remote PR. It never
// commits, pushes, or creates a replacement for an uncertain publication.
func (p *HostPublisher) Reconcile(ctx context.Context, request Request) (result Result, err error) {
	result = Result{Request: request, Stage: StageCandidate}
	if err = request.ValidateCandidate(); err != nil {
		return result, err
	}
	if p.Git == nil || p.PR == nil {
		return result, errors.New("local publisher is not configured")
	}
	if err = p.PR.validateHostRequest(request); err != nil {
		return result, err
	}
	defer func() { err = p.PR.redact(err) }()
	common, e := p.git(ctx, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if e != nil || canonical(common) != canonical(request.GitCommonDir) {
		return result, errors.Join(e, errors.New("recovery Git common directory changed"))
	}
	branch, e := p.git(ctx, "symbolic-ref", "--quiet", "--short", "HEAD")
	if e != nil || branch != request.Workspace.Branch {
		return result, errors.Join(e, errors.New("recovery branch changed"))
	}
	head, e := p.git(ctx, "rev-parse", "HEAD")
	if e != nil || !validCommit(head) {
		return result, errors.Join(e, errors.New("recovery commit is unavailable"))
	}
	tree, e := p.git(ctx, "rev-parse", "HEAD^{tree}")
	if e != nil || tree != request.IndexTree {
		return result, errors.Join(e, errors.New("recovery commit tree differs from saved staged content"))
	}
	if _, e = p.git(ctx, "merge-base", "--is-ancestor", request.StartingHead, "HEAD"); e != nil {
		return result, fmt.Errorf("recovery commit changed ancestry: %w", e)
	}
	result.Head, result.Stage = head, StageCommitted
	remote, e := p.git(ctx, "ls-remote", request.Remote, "refs/heads/"+request.Workspace.Branch)
	if e != nil || remote != head+"\trefs/heads/"+request.Workspace.Branch {
		return result, errors.Join(e, errors.New("fixed remote branch does not match the local candidate"))
	}
	result.Stage = StagePushed
	url, e := p.PR.Find(ctx, request, head)
	if e != nil || url == "" {
		return result, errors.Join(e, errors.New("fixed remote branch has no verified PR; publication remains uncertain"))
	}
	info, e := p.PR.Query(ctx, url)
	if e != nil || info.HeadRevision != head {
		return result, errors.Join(e, errors.New("recovered PR head does not match the fixed commit"))
	}
	result.Stage, result.PRURL, result.PR = StagePullRequest, url, &info
	return result, nil
}
