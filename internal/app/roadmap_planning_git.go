package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode"

	proc "github.com/erranliu/SteerLine/internal/execution/process"
	"github.com/erranliu/SteerLine/internal/hostcontract"
	"github.com/erranliu/SteerLine/internal/pathguard"
	roadmaps "github.com/erranliu/SteerLine/internal/roadmap"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
)

func validPlanningFileName(name string) bool {
	return len(name) > len(".md") && len(name) <= 240 && filepath.IsLocal(name) &&
		strings.EqualFold(filepath.Ext(name), ".md") && strings.TrimSpace(name) == name &&
		!strings.ContainsAny(name, `/\<>:"|?*`) && strings.IndexFunc(name, unicode.IsControl) < 0
}

func (p *planningPort) planningGit(ctx context.Context, root string, input []byte, env map[string]string, args ...string) ([]byte, error) {
	options := map[string]string{"GIT_TERMINAL_PROMPT": "0", "GIT_EDITOR": "true", "GIT_LITERAL_PATHSPECS": "1"}
	for key, value := range env {
		options[key] = value
	}
	runner := commandRunner{Dir: root, Input: input, Env: options, Timeout: positiveDuration(p.config.CommandTimeout, 30*time.Second), ProcessHost: p.config.ProcessHost}
	return runner.Output(ctx, p.config.Git, append([]string{"-c", "core.hooksPath=", "-c", "commit.gpgsign=false", "-c", "rerere.enabled=false", "-c", "user.name=SteerLine", "-c", "user.email=steerline@localhost"}, args...)...)
}

func safePlanningPath(root, name string) (string, error) {
	if name == "" || !filepath.IsLocal(name) || path.Clean(name) != name || strings.ContainsAny(name, "\\:\x00") {
		return "", errors.New("文件路径越界")
	}
	for _, part := range strings.Split(name, "/") {
		if strings.EqualFold(part, ".git") || strings.TrimRight(part, ". ") != part {
			return "", errors.New("文件路径不受支持")
		}
	}
	full := filepath.Join(root, filepath.FromSlash(name))
	resolved, err := pathguard.ResolveProspective(full)
	if err != nil || !strings.EqualFold(full, resolved) {
		return "", errors.New("文件路径发生重定向")
	}
	return full, nil
}

func (p *planningPort) treeEntry(ctx context.Context, root, commit, name string) (string, []byte, error) {
	raw, err := p.planningGit(ctx, root, nil, nil, "ls-tree", "-z", commit, "--", name)
	if err != nil || len(raw) == 0 {
		return "", nil, err
	}
	meta, got, _ := strings.Cut(strings.TrimSuffix(string(raw), "\x00"), "\t")
	fields := strings.Fields(meta)
	if got != name || len(fields) != 3 || (fields[0] != "100644" && fields[0] != "100755") {
		return "", nil, fmt.Errorf("无法审阅非普通文件 %s", name)
	}
	data, err := p.planningGit(ctx, root, nil, nil, "cat-file", "blob", fields[2])
	return fields[0], data, err
}

func (p *planningPort) planningChanges(ctx context.Context, task *roadmap.PlanningTask, base string, review bool) ([]roadmap.PlanningChange, error) {
	root := task.Run.Execution.Workspace
	delta, err := p.planningGit(ctx, root, nil, nil, "diff", "--no-renames", "--name-only", "-z", base, "--")
	if err != nil {
		return nil, err
	}
	untracked, err := p.planningGit(ctx, root, nil, nil, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	names := strings.Split(string(delta)+string(untracked), "\x00")
	slices.Sort(names)
	names = slices.Compact(names)
	changes := []roadmap.PlanningChange{}
	aliases := map[string]bool{}
	total := 0
	for _, name := range names {
		if name == "" {
			continue
		}
		alias := strings.ToLower(name)
		if aliases[alias] {
			return nil, errors.New("文件路径存在大小写冲突")
		}
		aliases[alias] = true
		if len(changes) >= 200 {
			return nil, errors.New("工作区差异超过审阅上限")
		}
		change, err := p.readPlanningChange(ctx, root, base, name)
		if err != nil {
			if !review {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			changes = append(changes, roadmap.PlanningChange{Path: name, Unavailable: err.Error()})
			continue
		}
		if change.Mode == change.OldMode && string(change.Before) == string(change.After) {
			continue
		}
		total += len(change.Before) + len(change.After)
		if total > roadmap.MaxBytes {
			return nil, errors.New("工作区差异超过审阅上限")
		}
		changes = append(changes, change)
	}
	return changes, nil
}

func (p *planningPort) readPlanningChange(ctx context.Context, root, base, name string) (roadmap.PlanningChange, error) {
	change := roadmap.PlanningChange{Path: name}
	full, err := safePlanningPath(root, name)
	if err != nil {
		return change, err
	}
	change.OldMode, change.Before, err = p.treeEntry(ctx, root, base, name)
	if err != nil {
		return change, err
	}
	info, err := os.Lstat(full)
	if os.IsNotExist(err) {
		return change, nil
	}
	if err != nil {
		return change, err
	}
	if !info.Mode().IsRegular() || info.Size() > roadmap.MaxBytes {
		return change, errors.New("文件类型或大小不支持发布")
	}
	change.After, err = os.ReadFile(full)
	if err != nil {
		return change, err
	}
	change.Mode = "100644"
	mode, err := p.planningGit(ctx, root, nil, nil, "ls-files", "--stage", "--", name)
	if strings.HasPrefix(string(mode), "100755 ") || (runtime.GOOS != "windows" && info.Mode().Perm()&0111 != 0) {
		change.Mode = "100755"
	}
	return change, err
}

func (p *planningPort) indexPlanningChanges(ctx context.Context, root, base, index string, changes []roadmap.PlanningChange) (string, error) {
	env := map[string]string{"GIT_INDEX_FILE": index}
	if _, err := p.planningGit(ctx, root, nil, env, "read-tree", base); err != nil {
		return "", err
	}
	for _, change := range changes {
		if change.Mode == "" {
			if _, err := p.planningGit(ctx, root, nil, env, "update-index", "--force-remove", "--", change.Path); err != nil {
				return "", err
			}
			continue
		}
		hash, err := p.planningGit(ctx, root, change.After, nil, "hash-object", "-w", "--stdin")
		if err != nil {
			return "", err
		}
		if _, err = p.planningGit(ctx, root, nil, env, "update-index", "--add", "--cacheinfo", change.Mode, strings.TrimSpace(string(hash)), change.Path); err != nil {
			return "", err
		}
	}
	tree, err := p.planningGit(ctx, root, nil, env, "write-tree")
	return strings.TrimSpace(string(tree)), err
}

func (p *planningPort) rebaseFacts(ctx context.Context, task *roadmap.PlanningTask) (branch, target, start string, files []string, err error) {
	root := task.Run.Execution.Workspace
	admin, err := p.planningGit(ctx, root, nil, nil, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", "", "", nil, err
	}
	for _, kind := range []string{"rebase-merge", "rebase-apply"} {
		dir := filepath.Join(strings.TrimSpace(string(admin)), kind)
		info, statErr := os.Lstat(dir)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil || !info.IsDir() || branch != "" {
			return "", "", "", nil, errors.New("原 Rebase 元数据目录无法核实")
		}
		head, readErr := os.ReadFile(filepath.Join(dir, "head-name"))
		if readErr != nil {
			return "", "", "", nil, readErr
		}
		onto, readErr := os.ReadFile(filepath.Join(dir, "onto"))
		if readErr != nil {
			return "", "", "", nil, readErr
		}
		origin, readErr := os.ReadFile(filepath.Join(dir, "orig-head"))
		if readErr != nil {
			return "", "", "", nil, readErr
		}
		branch, target, start = strings.TrimSpace(string(head)), strings.TrimSpace(string(onto)), strings.TrimSpace(string(origin))
		if branch == "" || target == "" || start == "" {
			return "", "", "", nil, errors.New("原 Rebase 元数据不完整")
		}
	}
	if branch == "" {
		for _, marker := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "sequencer"} {
			if _, markerErr := os.Lstat(filepath.Join(strings.TrimSpace(string(admin)), marker)); !os.IsNotExist(markerErr) {
				return "", "", "", nil, errors.New("工作区存在其他 Git 操作")
			}
		}
	}
	raw, err := p.planningGit(ctx, root, nil, nil, "diff", "--name-only", "--diff-filter=U", "-z", "--")
	if err != nil {
		return "", "", "", nil, err
	}
	files = []string{}
	for _, name := range strings.Split(string(raw), "\x00") {
		if name != "" {
			files = append(files, name)
		}
	}
	return
}

func (p *planningPort) checkPlanningRebase(ctx context.Context, task *roadmap.PlanningTask, target string) error {
	branch, onto, start, files, err := p.rebaseFacts(ctx, task)
	if err != nil {
		return err
	}
	original := "refs/heads/" + task.Run.Execution.Branch
	if branch != "" {
		var checkpoint string
		if task.Conflict != nil {
			checkpoint = task.Conflict.Checkpoint
		} else if task.Operation != nil {
			checkpoint = task.Operation.Checkpoint
		}
		if branch != original || onto != target || start != checkpoint {
			return errors.New("当前 Rebase 不属于原分支或目标")
		}
		if task.Conflict == nil {
			op := task.Operation
			task.Conflict = &roadmap.PlanningConflict{Target: target, Previous: op.Previous, Checkpoint: op.Checkpoint, IndexTree: op.IndexTree}
		}
		task.Conflict.Files = files
		task.Status = "conflict"
		return nil
	}
	root := task.Run.Execution.Workspace
	head, err := p.planningGit(ctx, root, nil, nil, "symbolic-ref", "HEAD")
	if err != nil || strings.TrimSpace(string(head)) != original || len(files) != 0 {
		return errors.New("原分支或未合并索引尚未恢复")
	}
	if _, err = p.planningGit(ctx, root, nil, nil, "merge-base", "--is-ancestor", target, "HEAD"); err != nil {
		return errors.New("Rebase 未到达原目标，保留现场供核对")
	}
	task.Base, task.Status, task.Conflict, task.Review = target, "ready", nil, nil
	if body, err := roadmaps.ReadRegular(root, task.Path); err == nil {
		task.Body = string(body)
	}
	return nil
}

func (p *planningPort) applyPlanningLease(ctx context.Context, task *roadmap.PlanningTask) error {
	op := task.Operation
	root := task.Run.Execution.Workspace
	var effectErr error
	switch {
	case op.Stage == "git_running":
		if len(op.Writer) != 0 {
			var writer hostcontract.ProcessIdentity
			if json.Unmarshal(op.Writer, &writer) != nil || runtime.GOOS != "windows" || !proc.WorkerTreeAbsent(writer) {
				return errors.New("原 Git 写者尚未核实，保留租约")
			}
		}
		if op.Kind == "abort" {
			effectErr = p.recoverPlanningAbort(ctx, task)
		} else {
			effectErr = p.checkPlanningRebase(ctx, task, op.Target)
		}
	case task.Merged != "" || task.StopRequested:
	case op.Kind == "rebase":
		effectErr = p.startPlanningRebase(ctx, task)
	case op.Kind == "abort":
		effectErr = p.abortPlanningRebase(ctx, task)
	case op.Kind == "review" || op.Kind == "publish":
		effectErr = p.capturePlanningReview(ctx, task)
	}
	if effectErr != nil {
		if op.Stage == "git_running" {
			// The managed Git call has returned, but the original effect still needs proof.
			op.Writer = nil
			return effectErr
		}
		task.Diagnostic = effectErr.Error()
		op.Diagnostic = task.Diagnostic
	}
	// A canceled process is waited and its owned job is closed by commandRunner.
	// A persisted running stage can only be recovered by checking Git facts.
	if op.Stage == "git_running" && ctx.Err() != nil {
		op.Writer = nil
		return ctx.Err()
	}
	if _, err := os.Stat(root); err != nil {
		return errors.New("原 worktree 无法核实，保留租约")
	}
	planControl(op, "workspace release", task.Run.Revision, map[string]any{"lease_id": op.Lease})
	return nil
}

func (p *planningPort) startPlanningRebase(ctx context.Context, task *roadmap.PlanningTask) error {
	op := task.Operation
	if op.Target == task.Base {
		task.Status = "ready"
		return nil
	}
	root := task.Run.Execution.Workspace
	branch, _, _, files, err := p.rebaseFacts(ctx, task)
	if err != nil || branch != "" || len(files) != 0 {
		return errors.New("已有 Git 操作或冲突")
	}
	head, err := p.planningGit(ctx, root, nil, nil, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	op.Previous = strings.TrimSpace(string(head))
	if _, err = p.planningGit(ctx, root, nil, nil, "fetch", "--no-tags", "--no-write-fetch-head", p.config.Repository, op.Target); err != nil {
		return err
	}
	for _, tip := range []string{op.Previous, op.Target} {
		if _, err = p.planningGit(ctx, root, nil, nil, "merge-base", "--is-ancestor", task.Base, tip); err != nil {
			return errors.New("规划基线与目标或原分支历史不一致")
		}
	}
	indexTree, err := p.planningGit(ctx, root, nil, nil, "write-tree")
	if err != nil {
		return err
	}
	op.IndexTree = strings.TrimSpace(string(indexTree))
	changes, err := p.planningChanges(ctx, task, op.Previous, false)
	if err != nil {
		return err
	}
	op.Checkpoint = op.Previous
	if len(changes) > 0 {
		dir, err := os.MkdirTemp(filepath.Dir(task.Material), "checkpoint-")
		if err != nil {
			return err
		}
		tree, err := p.indexPlanningChanges(ctx, root, op.Previous, filepath.Join(dir, "index"), changes)
		if err != nil {
			return err
		}
		commit, err := p.planningGit(ctx, root, nil, nil, "commit-tree", tree, "-p", op.Previous, "-m", "Checkpoint Roadmap planning work")
		if err != nil {
			return err
		}
		op.Checkpoint = strings.TrimSpace(string(commit))
	}
	if err = p.markPlanningGit(ctx, task); err != nil {
		return err
	}
	if _, err = p.planningGit(ctx, root, nil, nil, "update-ref", "refs/steerline/planning/"+task.ID+"/checkpoint", op.Checkpoint); err != nil {
		return err
	}
	if op.Checkpoint != op.Previous {
		if _, err = p.planningGit(ctx, root, nil, nil, "reset", "--mixed", op.Checkpoint); err != nil {
			return err
		}
	}
	_, gitErr := p.planningGit(ctx, root, nil, nil, "rebase", "--merge", "--no-autostash", "--onto", op.Target, task.Base)
	if err = p.checkPlanningRebase(ctx, task, op.Target); err != nil {
		return errors.Join(gitErr, err)
	}
	return nil
}

func (p *planningPort) markPlanningGit(ctx context.Context, task *roadmap.PlanningTask) error {
	writer, err := proc.CurrentIdentity()
	if err != nil {
		return err
	}
	task.Operation.Stage = "git_running"
	task.Operation.Writer, err = json.Marshal(writer)
	if err != nil {
		return err
	}
	return p.save(ctx, task)
}

func (p *planningPort) abortPlanningRebase(ctx context.Context, task *roadmap.PlanningTask) error {
	conflict := task.Conflict
	if conflict == nil {
		return errors.New("原冲突记录缺失")
	}
	branch, target, start, _, err := p.rebaseFacts(ctx, task)
	if err != nil || branch != "refs/heads/"+task.Run.Execution.Branch || target != conflict.Target || start != conflict.Checkpoint {
		return errors.New("原 Rebase 不可中止")
	}
	if err = p.markPlanningGit(ctx, task); err != nil {
		return err
	}
	root := task.Run.Execution.Workspace
	if _, err = p.planningGit(ctx, root, nil, nil, "rebase", "--abort"); err != nil {
		return err
	}
	return p.recoverPlanningAbort(ctx, task)
}

func (p *planningPort) recoverPlanningAbort(ctx context.Context, task *roadmap.PlanningTask) error {
	conflict := task.Conflict
	if conflict == nil {
		return errors.New("原冲突记录缺失")
	}
	root := task.Run.Execution.Workspace
	branch, _, _, files, err := p.rebaseFacts(ctx, task)
	if err != nil || branch != "" || len(files) != 0 {
		return errors.New("原中止尚未核实，保留检查点和租约")
	}
	ref, err := p.planningGit(ctx, root, nil, nil, "symbolic-ref", "HEAD")
	if err != nil || strings.TrimSpace(string(ref)) != "refs/heads/"+task.Run.Execution.Branch {
		return errors.New("中止后的原任务分支无法核实")
	}
	head, err := p.planningGit(ctx, root, nil, nil, "rev-parse", "HEAD")
	if err != nil || (strings.TrimSpace(string(head)) != conflict.Checkpoint && strings.TrimSpace(string(head)) != conflict.Previous) {
		return errors.New("中止后的 HEAD 与原检查点不一致")
	}
	changes, err := p.planningChanges(ctx, task, conflict.Checkpoint, false)
	if err != nil || len(changes) != 0 {
		return errors.New("中止后的工作区与原检查点不一致")
	}
	if err = p.markPlanningGit(ctx, task); err != nil {
		return err
	}
	if _, err = p.planningGit(ctx, root, nil, nil, "reset", "--soft", conflict.Previous); err != nil {
		return err
	}
	if _, err = p.planningGit(ctx, root, nil, nil, "read-tree", conflict.IndexTree); err != nil {
		return err
	}
	task.Conflict = nil
	task.Status = "ready"
	return nil
}

func (p *planningPort) capturePlanningReview(ctx context.Context, task *roadmap.PlanningTask) error {
	branch, _, _, unmerged, err := p.rebaseFacts(ctx, task)
	if err != nil || branch != "" || len(unmerged) != 0 {
		return errors.New("工作区 Git 操作尚未完成，暂不可发布")
	}
	body, err := roadmaps.ReadRegular(task.Run.Execution.Workspace, task.Path)
	if err != nil {
		return err
	}
	changes, err := p.planningChanges(ctx, task, task.Base, true)
	if err != nil {
		return err
	}
	task.Body = string(body)
	current := &roadmap.PlanningReview{Changes: changes, Digest: roadmap.ValueDigest(struct {
		Base    string
		Changes []roadmap.PlanningChange
	}{task.Base, changes})}
	task.Review = current
	op := task.Operation
	if op.Kind == "review" {
		return nil
	}
	if op.Digest != current.Digest {
		return errors.New("差异已变化，请重新审阅")
	}
	selected := map[string]bool{}
	for _, name := range op.Selected {
		if selected[name] {
			return errors.New("重复的确认文件")
		}
		selected[name] = true
	}
	if !selected[task.Path] {
		return errors.New("必须确认绑定的 Roadmap 草稿")
	}
	files := []roadmap.PlanningChange{}
	for _, change := range changes {
		if selected[change.Path] {
			if change.Unavailable != "" {
				return fmt.Errorf("无法发布所选文件 %s", change.Path)
			}
			files = append(files, change)
			delete(selected, change.Path)
		}
	}
	if len(selected) != 0 {
		return errors.New("确认文件已变化")
	}
	if task.Publication == nil {
		task.Publication = &roadmap.PlanningPublication{Branch: "codex/roadmap-planning-" + task.ID}
	}
	task.Publication.Versions = append(task.Publication.Versions, roadmap.PlanningVersion{Base: task.Base, Changes: files})
	op.Result = "prepared"
	return nil
}
