package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/app/host"
	mv "github.com/erranliu/SteerLine/internal/manualverification"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/google/uuid"
)

type prLaunchHistory struct {
	Latest   *projectLaunchRun  `json:"latest"`
	Retained []projectLaunchRun `json:"retained,omitempty"`
}

func (s *manualVerification) savePRRun(run projectLaunchRun) error {
	history := prLaunchHistory{Latest: s.prRun, Retained: slices.Clone(s.prRetained)}
	if i := slices.IndexFunc(history.Retained, func(old projectLaunchRun) bool { return old.ID == run.ID }); i >= 0 {
		if run.Removed {
			history.Retained = slices.Delete(history.Retained, i, i+1)
		} else {
			history.Retained[i] = run
		}
	} else {
		if history.Latest != nil && history.Latest.ID != run.ID && !history.Latest.Removed {
			history.Retained = append(history.Retained, *history.Latest)
		}
		history.Latest = &run
	}
	dir := s.prLaunchDir()
	if err := ordinaryVerificationPath(s.config.RunDir, dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	raw, err := json.Marshal(history)
	if err != nil {
		return err
	}
	if err := host.AtomicWrite(filepath.Join(dir, "runs.json"), raw); err != nil {
		return err
	}
	s.prRun, s.prRetained = history.Latest, history.Retained
	return nil
}

func (s *manualVerification) prRunByID(id string) *projectLaunchRun {
	if s.prRun != nil && s.prRun.ID == id {
		return s.prRun
	}
	if i := slices.IndexFunc(s.prRetained, func(run projectLaunchRun) bool { return run.ID == id }); i >= 0 {
		return &s.prRetained[i]
	}
	return nil
}

type launchPR struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	Branch string `json:"branch"`
	Head   string `json:"head"`
	State  string `json:"state"`
}

func (s *manualVerification) launchPR(fact roadmapPR) (launchPR, error) {
	number, err := s.config.PR.PRNumber(fact.URL)
	if err != nil || number != fact.Number || !taskmodel.ValidCommit(fact.Head.SHA) || fact.Head.Ref == "" ||
		(fact.State != "open" && fact.State != "closed") {
		return launchPR{}, errors.New("PR 仓库、状态或提交无法核实")
	}
	state := fact.State
	if fact.Merged {
		state = "merged"
	}
	return launchPR{Number: number, Title: fact.Title, URL: fact.URL, Head: fact.Head.SHA, Branch: fact.Head.Ref, State: state}, nil
}

func (s *manualVerification) recentLaunchPRs(ctx context.Context) ([]launchPR, error) {
	query := "?state=open&sort=updated&direction=desc&per_page=10"
	if s.config.PR.Kind == "gitea" {
		query = "?state=open&sort=recentupdate&limit=10"
	}
	var facts []roadmapPR
	if err := s.config.repositoryPRRequest(ctx, "GET", "repos/"+s.config.PR.Repository+"/pulls"+query, nil, &facts); err != nil {
		return nil, err
	}
	rows := make([]launchPR, 0, min(len(facts), 10))
	for _, fact := range facts[:min(len(facts), 10)] {
		pr, err := s.launchPR(fact)
		if err != nil {
			return nil, err
		}
		rows = append(rows, pr)
	}
	return rows, nil
}

func (s *manualVerification) preparePRLaunch(ctx context.Context, m *projectMaintenance, number int, expected string) (launchPR, []mv.ProjectLaunch, error) {
	if m == nil || number <= 0 {
		return launchPR{}, nil, errors.New("项目或 PR 编号无效")
	}
	remote, err := s.git(ctx, "remote", "get-url", "--", s.config.Remote)
	if err != nil {
		return launchPR{}, nil, err
	}
	remoteURL := strings.TrimSpace(string(remote))
	if err := s.config.PR.ValidateRemote(remoteURL); err != nil {
		return launchPR{}, nil, err
	}
	var fact roadmapPR
	if err := s.config.repositoryPRRequest(ctx, "GET", "repos/"+s.config.PR.Repository+"/pulls/"+strconv.Itoa(number), nil, &fact); err != nil {
		return launchPR{}, nil, err
	}
	pr, err := s.launchPR(fact)
	if err != nil || pr.Number != number {
		return launchPR{}, nil, errors.New("PR 仓库或编号与请求不一致")
	}
	if expected != "" && pr.Head != expected {
		return pr, nil, errors.New("PR Head 已变化，请重新选择入口")
	}
	ref := fmt.Sprintf("refs/pull/%d/head", number)
	refID := strings.ReplaceAll(uuid.NewString(), "-", "")
	err = s.fetchVerificationHead(ctx, remoteURL, ref, pr.Head, refID)
	if err != nil && pr.State != "open" && ctx.Err() == nil {
		// The platform may stop advertising the PR ref after closure.
		err = s.fetchVerificationHead(ctx, remoteURL, pr.Head, pr.Head, refID)
	}
	if err != nil {
		return pr, nil, err
	}
	entries, err := projectLaunchEntries(ctx, m, pr.Head)
	return pr, entries, err
}

func (s *manualVerification) readPRLaunch(ctx context.Context, m *projectMaintenance, route string) (any, error) {
	const base = "/manual-verification/pulls"
	if route == base {
		return s.recentLaunchPRs(ctx)
	}
	path := strings.TrimPrefix(route, base+"/")
	if path == "run" {
		s.mu.Lock()
		defer s.mu.Unlock()
		result := prLaunchHistory{Retained: slices.Clone(s.prRetained)}
		if s.prRun != nil {
			run := *s.prRun
			run.Process = nil
			result.Latest = &run
		}
		for i := range result.Retained {
			result.Retained[i].Process = nil
		}
		return result, nil
	}
	if id, ok := strings.CutPrefix(path, "logs/"); ok && projectLaunchRunID.MatchString(id) {
		s.mu.Lock()
		run := s.prRunByID(id)
		s.mu.Unlock()
		if run == nil || run.ID != id {
			return nil, errors.New("找不到 PR 启动日志")
		}
		return s.readLaunchLogs(filepath.Join(s.prLaunchDir(), id))
	}
	number, err := strconv.Atoi(path)
	if err != nil || number <= 0 {
		return nil, errors.New("PR 编号无效")
	}
	pr, entries, err := s.preparePRLaunch(ctx, m, number, "")
	if pr.Number == 0 {
		return nil, err
	}
	result := struct {
		PR         launchPR           `json:"pr"`
		Entries    []mv.ProjectLaunch `json:"entries"`
		Diagnostic string             `json:"diagnostic,omitempty"`
	}{PR: pr, Entries: entries}
	if err != nil {
		result.Diagnostic = err.Error()
	}
	return result, nil
}

func (s *manualVerification) startPRLaunch(ctx context.Context, m *projectMaintenance, id string, number int, head, entryID string) error {
	s.mu.Lock()
	previous := s.prRun
	busy := previous != nil && (previous.Active() || s.stops[previous.ID] != nil)
	s.mu.Unlock()
	if busy {
		return errors.New("请先停止原 PR 启动")
	}
	_, entries, err := s.preparePRLaunch(ctx, m, number, head)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.ID != entryID {
			continue
		}
		worktree, err := s.newVerificationWorktree(id)
		if err != nil {
			return err
		}
		run := projectLaunchRun{Attempt: mv.Attempt{ID: id, Head: head, Worktree: worktree, Source: mv.SourcePR, Status: "preparing"}, Entry: entry.ID, Name: entry.Name, PR: number}
		launchCtx, cancel := context.WithCancel(s.ctx)
		s.mu.Lock()
		err = s.saveProjectLaunch(run)
		if err == nil {
			s.stops[id] = cancel
		}
		s.mu.Unlock()
		if err != nil {
			cancel()
			return err
		}
		s.workers.Go(func() {
			defer cancel()
			s.runPRLaunch(launchCtx, run, entry.Launch)
		})
		return nil
	}
	return errors.New("PR 启动入口已变化，请重新选择")
}

func (s *manualVerification) runPRLaunch(ctx context.Context, run projectLaunchRun, launch mv.Launch) {
	patch := func(update func(*mv.Attempt)) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		update(&run.Attempt)
		err := s.saveProjectLaunch(run)
		if err != nil {
			current := run
			s.prRun = &current
		}
		return err
	}
	logDir := filepath.Join(s.prLaunchDir(), run.ID)
	err := ordinaryVerificationPath(s.config.RunDir, logDir)
	if err == nil {
		err = os.Mkdir(logDir, 0700)
	}
	if err == nil {
		err = s.createVerificationWorktree(ctx, run.Attempt)
	}
	if err == nil {
		err = s.runLaunch(ctx, run.Attempt, launch, run.Worktree, logDir, true, patch)
	}
	if err != nil && run.Active() && run.Status != "unknown" {
		run.Status, run.Diagnostic = "failed", err.Error()
		if errors.Is(err, context.Canceled) {
			run.Status = "stopped"
		}
	}
	if !run.Active() {
		s.cleanupPRLaunch(&run)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.saveProjectLaunch(run); err != nil {
		run.Diagnostic = strings.TrimSpace(run.Diagnostic + " 保存启动状态失败：" + err.Error())
		s.prRun = &run
	}
	delete(s.stops, run.ID)
}

func (s *manualVerification) cleanupPRLaunch(run *projectLaunchRun) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	err := s.removePRWorktree(ctx, run.Attempt)
	run.Removed = err == nil
	run.CleanupDiagnostic = ""
	if err != nil {
		run.CleanupDiagnostic = "已保留临时 worktree：" + err.Error()
	}
}

func (s *manualVerification) removePRWorktree(ctx context.Context, a mv.Attempt) error {
	if err := s.verifyWorktreePath(a); err != nil {
		return err
	}
	if _, err := os.Lstat(a.Worktree); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if err := s.verifyWorktree(ctx, a, false); err != nil {
		return err
	}
	if _, err := s.git(ctx, "worktree", "remove", a.Worktree); err != nil {
		return err
	}
	// The checkout is separate from the logs. Remove only its empty parent.
	_ = os.Remove(filepath.Dir(a.Worktree))
	return nil
}

func (s *manualVerification) finishPRLaunch(ctx context.Context, id string, stop bool) error {
	s.mu.Lock()
	original := s.prRunByID(id)
	if original == nil {
		s.mu.Unlock()
		return errors.New("原 PR 启动记录已变化")
	}
	run := *original
	if cancel := s.stops[id]; cancel != nil {
		if stop {
			cancel()
		}
		s.mu.Unlock()
		if !stop {
			return errors.New("原 PR 启动尚在运行或清理")
		}
		return nil
	}
	s.mu.Unlock()
	if run.Active() {
		if !verificationTreeAbsent(run.Attempt) {
			return errors.New("原 PR 启动进程尚未核实")
		}
		run.Status = "stopped"
	}
	if !run.Removed {
		s.cleanupPRLaunch(&run)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveProjectLaunch(run)
}

func (s *manualVerification) recoverPRLaunch(ctx context.Context) error {
	name := filepath.Join(s.prLaunchDir(), "runs.json")
	if err := ordinaryVerificationPath(s.config.RunDir, name); err != nil {
		return err
	}
	raw, err := os.ReadFile(name)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var history prLaunchHistory
	if err := json.Unmarshal(raw, &history); err != nil {
		return err
	}
	if history.Latest == nil {
		return errors.New("无效的 PR 启动记录")
	}
	run := *history.Latest
	for _, item := range append(slices.Clone(history.Retained), run) {
		if item.PR <= 0 || !taskmodel.ValidCommit(item.Head) || !projectLaunchRunID.MatchString(item.ID) || !filepath.IsAbs(item.Worktree) {
			return errors.New("无效的 PR 启动记录")
		}
	}
	s.prRun, s.prRetained = &run, history.Retained
	if run.Active() && !verificationTreeAbsent(run.Attempt) {
		run.Status = "unknown"
		return s.saveProjectLaunch(run)
	}
	if !run.Removed {
		return s.finishPRLaunch(ctx, run.ID, false)
	}
	return nil
}
