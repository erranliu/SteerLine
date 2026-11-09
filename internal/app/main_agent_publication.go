package app

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"

	ma "github.com/erranliu/SteerLine/internal/mainagent/model"
)

func (p *mainAgentPort) publish(ctx context.Context, id, request string, expected uint64, review string) (ma.Record, error) {
	p.publicationMu.Lock()
	defer p.publicationMu.Unlock()
	r, err := p.store.Analysis(ctx, p.config.Identity(), id)
	if err != nil {
		return r, err
	}
	if r.Publication != nil {
		if r.Publication.RequestID != request || r.Publication.Review != review {
			return r, errors.New("原文档发布已经登记，请核实原请求")
		}
		return p.observePublicationLocked(ctx, r)
	}
	if r.Revision != expected || r.State != "review" || r.Result == nil || r.Result.Disposition != "revision" || review != ma.Review(r) {
		return r, errors.New("stale_revision: 请重新审阅完整材料差异")
	}
	if err = ma.Validate(r, *r.Result); err != nil {
		return r, err
	}
	candidate, err := p.store.Candidate(ctx, p.config.Identity(), r.Context.CandidateID)
	if err != nil {
		return r, err
	}
	graph, err := p.store.TaskGraphSnapshot(ctx, p.config.Identity())
	if err != nil {
		return r, err
	}
	task, ok := graph.ByID[r.Context.TaskID]
	if !ok || task.Status != "in_progress" || task.Version != r.Context.TaskVersion || candidate.Participation != "active" || candidate.StopRequested || candidate.ContractDigest != r.Context.ContractDigest || candidate.Contract.SpecRef.Path != r.Context.SpecPath || candidate.Contract.PlanRef.Path != r.Context.PlanPath {
		return r, errors.New("stale_revision: 原任务或候选参与资格已变化")
	}
	baseline, err := p.config.runtimeMaintenance.check(ctx)
	if err != nil {
		return r, err
	}
	if baseline.Commit != r.Context.RepositoryCommit {
		return r, errors.New("stale_revision: 目标分支已变化，请重新分析")
	}
	for _, d := range r.Context.Current {
		body, e := p.config.runtimeMaintenance.read(ctx, baseline.Commit, d.Path)
		if e != nil || ma.Digest(body) != d.SHA256 {
			return r, errors.New("stale_revision: 当前材料与审阅版本不同")
		}
	}
	if _, err = p.config.TaskRepositoryTarget(ctx); err != nil {
		return r, err
	}
	if p.config.PR.Kind != "gitea" && !(p.config.PR.Kind == "command" && strings.TrimRight(p.config.PR.BaseURL, "/") == "https://github.com") {
		return r, errors.New("当前 provider 不支持材料 PR")
	}
	r.Publication = &ma.Publication{RequestID: request, Review: review, Base: baseline.Commit, Branch: "codex/material-analysis-" + r.ID, State: "preparing"}
	r, err = p.service.Save(ctx, r)
	if err != nil {
		return r, err
	}
	if err = p.preparePublication(ctx, &r); err != nil {
		r.Publication.Diagnostic = err.Error()
		r, _ = p.service.Save(context.WithoutCancel(ctx), r)
		return r, err
	}
	r.Publication.State = "push_unknown"
	r, err = p.service.Save(ctx, r)
	if err != nil {
		return r, err
	}
	pub := r.Publication
	if _, err = p.config.runtimeMaintenance.git(ctx, "push", "--force-with-lease=refs/heads/"+pub.Branch+":", p.config.Remote, pub.Commit+":refs/heads/"+pub.Branch); err != nil {
		return r, errors.New("材料分支推送结果待核实；不要重复提交")
	}
	r.Publication.State = "pushed"
	r, err = p.service.Save(ctx, r)
	if err != nil {
		return r, err
	}
	provider := documentProvider{p.config}
	found, err := provider.findPR(ctx, pub.Branch)
	if err != nil {
		return r, err
	}
	if found != nil {
		return p.acceptPublication(ctx, r, *found)
	}
	r.Publication.State = "pr_unknown"
	r, err = p.service.Save(ctx, r)
	if err != nil {
		return r, err
	}
	var fact roadmapPR
	err = provider.provider(ctx, "POST", "repos/"+p.config.PR.Repository+"/pulls", map[string]any{"title": "澄清任务 #" + r.Context.TaskID + " 的材料要求", "head": pub.Branch, "base": p.config.Branch, "body": r.Result.Summary + "\n\n" + r.Result.Recommendation, "draft": true}, &fact)
	if err != nil {
		return r, errors.New("材料 PR 创建结果待核实；仅查询原分支，不重复创建")
	}
	return p.acceptPublication(ctx, r, fact)
}
func (p *mainAgentPort) acceptPublication(ctx context.Context, r ma.Record, f roadmapPR) (ma.Record, error) {
	if f.URL == "" || f.Number <= 0 || f.Head.Ref != r.Publication.Branch || f.Head.SHA != r.Publication.Commit || f.Head.Repo.FullName != p.config.PR.Repository || f.Base.Ref != p.config.Branch {
		return r, errors.New("原材料 PR 身份或提交未核实")
	}
	if _, err := p.config.PR.PRNumber(f.URL); err != nil {
		return r, err
	}
	r.Publication.URL, r.Publication.State = f.URL, "published"
	return p.service.Save(ctx, r)
}
func (p *mainAgentPort) observePublication(ctx context.Context, r ma.Record) (ma.Record, error) {
	p.publicationMu.Lock()
	defer p.publicationMu.Unlock()
	current, err := p.store.Analysis(ctx, p.config.Identity(), r.ID)
	if err != nil {
		return r, err
	}
	return p.observePublicationLocked(ctx, current)
}
func (p *mainAgentPort) observePublicationLocked(ctx context.Context, r ma.Record) (ma.Record, error) {
	if r.Publication == nil {
		return r, errors.New("没有原材料发布请求")
	}
	if r.Publication.State == "published" {
		return r, nil
	}
	if r.Publication.State == "push_unknown" {
		raw, err := p.config.runtimeMaintenance.git(ctx, "ls-remote", "--refs", p.config.Remote, "refs/heads/"+r.Publication.Branch)
		if err != nil || strings.TrimSpace(string(raw)) != r.Publication.Commit+"\trefs/heads/"+r.Publication.Branch {
			return r, errors.New("原推送尚未核实，不重复推送")
		}
	}
	f, err := (documentProvider{p.config}).findPR(ctx, r.Publication.Branch)
	if err != nil {
		return r, err
	}
	if f == nil {
		return r, errors.New("原 PR 尚未核实，不自动补发创建")
	}
	return p.acceptPublication(ctx, r, *f)
}
func (p *mainAgentPort) preparePublication(ctx context.Context, r *ma.Record) error {
	m := p.config.runtimeMaintenance
	pub := r.Publication
	dir := filepath.Join(p.config.RunDir, "material-publications", r.ID)
	if err := makePublicationParent(dir); err != nil {
		return err
	}
	if !publicationDirectoryExists(dir) {
		if _, err := m.git(ctx, "worktree", "add", "-b", pub.Branch, dir, pub.Base); err != nil {
			return err
		}
	}
	git := func(args ...string) ([]byte, error) { return m.git(ctx, append([]string{"-C", dir}, args...)...) }
	common, err := git("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || canonicalPath(strings.TrimSpace(string(common))) != canonicalPath(p.config.Identity()) {
		return errors.New("材料 worktree 仓库身份不同")
	}
	branch, err := git("symbolic-ref", "--short", "HEAD")
	if err != nil || strings.TrimSpace(string(branch)) != pub.Branch {
		return errors.New("材料 worktree 分支身份不同")
	}
	head, err := git("rev-parse", "HEAD^{commit}")
	if err != nil || strings.TrimSpace(string(head)) != pub.Base {
		return errors.New("材料 worktree 基线已变化")
	}
	status, err := git("status", "--porcelain")
	if err != nil || strings.TrimSpace(string(status)) != "" {
		return errors.New("材料 worktree 不干净，不覆盖已有修改")
	}
	keys := []string{}
	for _, change := range r.Result.Changes {
		if err = writeRoadmapFile(dir, change.Path, change.Body); err != nil {
			return err
		}
		keys = append(keys, change.Path)
	}
	sort.Strings(keys)
	if _, err = git(append([]string{"add", "--"}, keys...)...); err != nil {
		return err
	}
	staged, err := git("diff", "--cached", "--name-only", "-z")
	if err != nil {
		return err
	}
	actual := strings.Split(strings.TrimSuffix(string(staged), "\x00"), "\x00")
	sort.Strings(actual)
	if strings.Join(actual, "\n") != strings.Join(keys, "\n") {
		return errors.New("材料提交含未审阅文件或无有效差异")
	}
	if _, err = git("commit", "-m", "Clarify material requirements for task "+r.Context.TaskID); err != nil {
		return err
	}
	commit, err := git("rev-parse", "HEAD^{commit}")
	if err != nil {
		return err
	}
	pub.Commit = strings.TrimSpace(string(commit))
	parents, err := git("rev-list", "--parents", "-n", "1", pub.Commit)
	if err != nil || strings.Join(strings.Fields(string(parents)), " ") != pub.Commit+" "+pub.Base {
		return errors.New("材料提交的父基线不符")
	}
	changed, err := git("diff", "--name-only", "-z", pub.Base, pub.Commit)
	if err != nil {
		return err
	}
	committed := strings.Split(strings.TrimSuffix(string(changed), "\x00"), "\x00")
	sort.Strings(committed)
	if strings.Join(committed, "\n") != strings.Join(keys, "\n") {
		return errors.New("材料提交包含未审阅文件，不发布")
	}
	for _, change := range r.Result.Changes {
		body, err := m.read(ctx, pub.Commit, change.Path)
		if err != nil || string(body) != change.Body {
			return errors.New("材料提交正文与审阅内容不同")
		}
	}
	return nil
}
