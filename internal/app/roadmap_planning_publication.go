package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	plan "github.com/erranliu/SteerLine/internal/roadmap/model"
)

func (p *planningPort) publishPlanning(ctx context.Context, task *plan.PlanningTask) error {
	op, pub := task.Operation, task.Publication
	if task.Merged != "" {
		task.Operation = nil
		return nil
	}
	if pub == nil || len(pub.Versions) == 0 {
		return errors.New("缺少确认的发布快照")
	}
	version := &pub.Versions[len(pub.Versions)-1]
	git := p.config.runtimeMaintenance.git
	switch op.Stage {
	case "publish_ready":
		target, err := p.config.runtimeMaintenance.check(ctx)
		if err != nil {
			return err
		}
		if target.Commit != version.Base {
			task.Operation = nil
			return errors.New("目标分支已更新，请 Rebase 后重新审阅")
		}
		if pub.URL != "" {
			fact, err := p.prFact(ctx, pub.URL)
			if err != nil {
				return err
			}
			if fact.Merged {
				return p.adoptPlanningMerge(ctx, task, fact)
			}
			if fact.State != "open" || fact.Head.SHA != pub.Head || fact.Head.Ref != pub.Branch || fact.Base.Ref != p.config.Branch {
				task.Operation = nil
				return errors.New("原 PR 已关闭或发布分支已变化")
			}
		}
		head, err := p.preparePlanningCommit(ctx, task, version)
		if err != nil {
			return err
		}
		version.Head = head
		op.Stage = "push_unknown"
		if err = p.save(ctx, task); err != nil {
			return err
		}
		fallthrough
	case "push_unknown":
		ref := "refs/heads/" + pub.Branch
		got, err := git(ctx, "ls-remote", "--refs", p.config.Remote, ref)
		if err != nil {
			return errors.New("原推送结果待核实")
		}
		if observed := strings.TrimSpace(string(got)); observed != version.Head+"\t"+ref {
			expected := ""
			if pub.Head != "" {
				expected = pub.Head + "\t" + ref
			}
			if observed != expected {
				return errors.New("原发布分支已变化，推送结果待核实")
			}
			target, err := p.config.runtimeMaintenance.check(ctx)
			if err != nil {
				return err
			}
			if target.Rewritten || target.Commit != version.Base {
				task.Operation, task.Review = nil, nil
				return errors.New("目标分支已更新，请 Rebase 后重新审阅")
			}
			if _, err := git(ctx, "push", "--force-with-lease="+ref+":"+pub.Head, p.config.Remote, version.Head+":"+ref); err != nil {
				return errors.New("原推送结果待核实")
			}
		}
		op.Stage = "pr_ready"
	case "pr_ready":
		fact, err := p.findPR(ctx, pub.Branch)
		if err != nil {
			return err
		}
		if fact == nil {
			if pub.URL != "" {
				return errors.New("原 PR 无法核实")
			}
			op.Stage = "pr_unknown"
			if err = p.save(ctx, task); err != nil {
				return err
			}
			fact = &roadmapPR{}
			err = p.provider(ctx, "POST", "repos/"+p.config.PR.Repository+"/pulls", map[string]any{"title": task.Title, "head": pub.Branch, "base": p.config.Branch}, fact)
			if err != nil {
				return errors.New("原 PR 创建结果待核实")
			}
		}
		return p.acceptPlanningPR(ctx, task, *fact)
	case "pr_unknown":
		fact, err := p.findPR(ctx, pub.Branch)
		if err != nil || fact == nil {
			return errors.New("原 PR 创建结果待核实")
		}
		return p.acceptPlanningPR(ctx, task, *fact)
	}
	return nil
}

func (p *planningPort) preparePlanningCommit(ctx context.Context, task *plan.PlanningTask, version *plan.PlanningVersion) (string, error) {
	dir, err := os.MkdirTemp(filepath.Dir(task.Material), "publication-")
	if err != nil {
		return "", err
	}
	root := filepath.Join(dir, "worktree")
	git := p.config.runtimeMaintenance.git
	if _, err = git(ctx, "-c", "core.hooksPath=", "worktree", "add", "--detach", root, version.Base); err != nil {
		return "", err
	}
	defer func() { _, _ = git(context.WithoutCancel(ctx), "worktree", "remove", root) }()
	tree, err := p.indexPlanningChanges(ctx, root, version.Base, filepath.Join(dir, "index"), version.Changes)
	if err != nil {
		return "", err
	}
	raw, err := p.planningGit(ctx, root, nil, nil, "commit-tree", tree, "-p", version.Base, "-m", "Plan "+task.Title)
	if err != nil {
		return "", err
	}
	head := strings.TrimSpace(string(raw))
	diff, err := p.planningGit(ctx, root, nil, nil, "diff-tree", "--no-commit-id", "--name-only", "--no-renames", "-r", "-z", version.Base, head)
	if err != nil {
		return "", err
	}
	names := strings.Split(strings.TrimSuffix(string(diff), "\x00"), "\x00")
	if len(names) != len(version.Changes) {
		return "", errors.New("发布提交的文件集合与确认不一致")
	}
	for i, change := range version.Changes {
		if names[i] != change.Path {
			return "", errors.New("发布提交包含未确认文件")
		}
		mode, body, err := p.treeEntry(ctx, root, head, change.Path)
		if err != nil || mode != change.Mode || !bytes.Equal(body, change.After) {
			return "", fmt.Errorf("发布内容与确认不一致：%s", change.Path)
		}
	}
	return head, nil
}

func (p *planningPort) acceptPlanningPR(ctx context.Context, task *plan.PlanningTask, fact roadmapPR) error {
	pub := task.Publication
	if fact.Head.Ref != pub.Branch || fact.Head.Repo.FullName != p.config.PR.Repository || fact.Base.Ref != p.config.Branch || fact.URL == "" || (pub.URL != "" && pub.URL != fact.URL) {
		return errors.New("原规划 PR 身份无法核实")
	}
	if _, err := p.config.PR.PRNumber(fact.URL); err != nil {
		return err
	}
	pub.URL = fact.URL
	if fact.Merged {
		return p.adoptPlanningMerge(ctx, task, fact)
	}
	version := pub.Versions[len(pub.Versions)-1]
	if fact.Head.SHA != version.Head || fact.State != "open" {
		return errors.New("原 PR 已关闭或 head 无法核实")
	}
	pub.Head = fact.Head.SHA
	task.Status = "awaiting_merge"
	task.Operation = nil
	task.Diagnostic = ""
	return nil
}

func (p *planningPort) observePlanningPR(ctx context.Context, task *plan.PlanningTask) error {
	fact, err := p.prFact(ctx, task.Publication.URL)
	if err != nil {
		return err
	}
	if fact.Head.Ref != task.Publication.Branch || fact.Base.Ref != p.config.Branch {
		return errors.New("原规划 PR 目标已变化")
	}
	if fact.Merged {
		return p.adoptPlanningMerge(ctx, task, fact)
	}
	if fact.State != "open" {
		return errors.New("原规划 PR 已关闭，尚未合并")
	}
	return nil
}

func (p *planningPort) adoptPlanningMerge(ctx context.Context, task *plan.PlanningTask, fact roadmapPR) error {
	pub := task.Publication
	if !fact.Merged || fact.MergeCommit == "" || fact.Head.Ref != pub.Branch || fact.Base.Ref != p.config.Branch {
		return errors.New("原 PR 合并事实无法核实")
	}
	var version *plan.PlanningVersion
	for i := range pub.Versions {
		item := &pub.Versions[i]
		if item.Head != "" && item.Head == fact.Head.SHA {
			version = item
			break
		}
	}
	if version == nil {
		return errors.New("实际合入的 PR head 未经审阅确认")
	}
	target, err := p.config.runtimeMaintenance.check(ctx)
	if err != nil {
		return err
	}
	if _, err = p.config.runtimeMaintenance.git(ctx, "merge-base", "--is-ancestor", fact.MergeCommit, target.Commit); err != nil {
		return errors.New("合并提交尚未进入目标分支")
	}
	merged := *task
	for _, change := range version.Changes {
		mode, body, err := p.treeEntry(ctx, p.config.Repository, fact.MergeCommit, change.Path)
		if err != nil || mode != change.Mode || !bytes.Equal(body, change.After) {
			return fmt.Errorf("合并文件与确认版本不一致：%s", change.Path)
		}
		if change.Path == task.Path {
			merged.Body = string(body)
		}
	}
	merged.Merged = fact.MergeCommit
	merged.Status = "merged_pending_sync"
	merged.Diagnostic = ""
	if err := p.save(ctx, &merged); err != nil {
		return err
	}
	*task = merged
	return nil
}

func (p *planningPort) handoffPlanning(ctx context.Context, task *plan.PlanningTask) error {
	if task.SourceID != "" {
		return nil
	}
	m := p.config.runtimeMaintenance
	branch, err := m.git(ctx, "symbolic-ref", "HEAD")
	if err != nil || strings.TrimSpace(string(branch)) != "refs/heads/"+p.config.Branch {
		return errors.New("已合并，项目工作树未位于目标分支")
	}
	source, err := p.source(ctx, task.Path)
	if err != nil {
		return errors.New("已合并，待同步草稿文件")
	}
	if source.Modified {
		return errors.New("已合并，草稿有本地修改，待同步")
	}
	if _, err = m.git(ctx, "merge-base", "--is-ancestor", task.Merged, source.Head); err != nil {
		return errors.New("已合并，待同步目标分支")
	}
	remote, err := m.check(ctx)
	if err != nil || remote.Commit == "" {
		return errors.New("已合并，目标分支版本待核实")
	}
	if _, err = m.git(ctx, "merge-base", "--is-ancestor", source.Head, remote.Commit); err != nil {
		return errors.New("已合并，本地目标分支有独有提交")
	}
	current, err := m.git(ctx, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(string(current)) != source.Head {
		return errors.New("项目工作树已变化，待重新核实")
	}
	branch, err = m.git(ctx, "symbolic-ref", "HEAD")
	if err != nil || strings.TrimSpace(string(branch)) != "refs/heads/"+p.config.Branch {
		return errors.New("已合并，项目工作树未位于目标分支")
	}
	task.SourceID = source.ID
	task.Status = "handed_off"
	task.Diagnostic = ""
	return nil
}
