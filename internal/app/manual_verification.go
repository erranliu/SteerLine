package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/backend"
	processfacts "github.com/erranliu/SteerLine/internal/execution/process"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	mv "github.com/erranliu/SteerLine/internal/manualverification"
	"github.com/erranliu/SteerLine/internal/pathguard"
	"github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type manualVerification struct {
	config    Config
	store     *sqlite.SQLiteStore
	ctx       context.Context
	cancel    context.CancelFunc
	workers   sync.WaitGroup
	mu        sync.Mutex
	commandMu sync.Mutex
	portMu    sync.Mutex
	ports     map[int]bool
	stops     map[string]context.CancelFunc
}

func newManualVerification(c Config, store *sqlite.SQLiteStore) *manualVerification {
	ctx, cancel := context.WithCancel(context.Background())
	return &manualVerification{config: c, store: store, ctx: ctx, cancel: cancel, stops: map[string]context.CancelFunc{}, ports: map[int]bool{}}
}

func (s *manualVerification) close() {
	s.cancel()
	s.workers.Wait()
}

func (s *manualVerification) worktree(id string) string {
	return filepath.Join(s.config.RunDir, "manual-verification", id, "worktree")
}

func (s *manualVerification) git(ctx context.Context, args ...string) ([]byte, error) {
	return (commandRunner{Dir: s.config.Repository, Timeout: 30 * time.Second, ProcessHost: s.config.ProcessHost}).Output(ctx, s.config.Git, append([]string{"--no-replace-objects"}, args...)...)
}

func (s *manualVerification) refresh(ctx context.Context, r mv.Record) (ports.RunnerPRTarget, string, error) {
	var target ports.RunnerPRTarget
	c, err := s.store.Candidate(ctx, r.Project, r.Candidate)
	if err != nil {
		return target, "", err
	}
	if !r.Merged && (c.Participation != "active" || c.StopRequested) {
		return target, "", errors.New("原候选已退出参与")
	}
	target, err = ports.BindRunnerPR(c, c.Contract.Branch)
	if err != nil {
		return target, "", err
	}
	if target.URL != r.PR || target.Provider.Kind != "command" || strings.TrimRight(target.Provider.BaseURL, "/") != "https://github.com" {
		return target, "", errors.New("核验仅支持已绑定的 GitHub PR")
	}
	query := publication.QueryClient{Command: commandRunner{Timeout: 30 * time.Second, ProcessHost: s.config.ProcessHost}, Timeout: 30 * time.Second}
	observation := ports.RunnerPRObservation{CandidateID: c.ID, RunID: c.RunID, CandidateRevision: c.Revision, StartedAt: time.Now().UTC()}
	observation.Fact, err = query.Query(ctx, target.Provider, target.URL)
	if err == nil && !target.Matches(observation.Fact) {
		err = errors.New("PR 身份与原候选不同")
	}
	observation.CheckedAt = time.Now().UTC()
	if storeErr := s.store.ObserveManualPR(ctx, r.Project, observation, err); storeErr != nil {
		return target, "", storeErr
	}
	if err != nil {
		return target, "", err
	}
	fact := observation.Fact
	if r.Merged && fact.State == "merged" && fact.Merged {
		return target, c.Contract.Repository, nil
	}
	if fact.State != "open" || fact.Merged || fact.HeadRevision != r.Head {
		return target, "", errors.New("PR 已关闭或 Head 已变化，请刷新后重试")
	}
	if observation.Fact.Draft == nil {
		return target, "", errors.New("PR 平台未返回草稿状态")
	}
	return target, c.Contract.Repository, nil
}

func (s *manualVerification) remoteVerificationHead(ctx context.Context, target ports.RunnerPRTarget, remote string) (string, error) {
	if err := target.Provider.ValidateRemote(remote); err != nil {
		return "", err
	}
	ref := "refs/heads/" + target.Base
	out, err := s.git(ctx, "ls-remote", "--exit-code", "--refs", "--", remote, ref)
	fields := strings.Fields(string(out))
	if err != nil || len(fields) != 2 || fields[1] != ref || !taskmodel.ValidCommit(fields[0]) {
		return "", errors.New("无法确定远端目标分支 Head")
	}
	return fields[0], nil
}

func (s *manualVerification) patch(candidate, id string, update func(*mv.Attempt)) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.store.UpdateManualVerification(ctx, s.config.Identity(), candidate, func(r *mv.Record) error {
		i, err := r.Attempt(id)
		if err != nil {
			return err
		}
		update(&r.Attempts[i])
		return nil
	})
}

func (s *manualVerification) launch(r mv.Record, attempt mv.Attempt, target ports.RunnerPRTarget, remote string) {
	ctx, cancel := context.WithCancel(s.ctx)
	s.mu.Lock()
	s.stops[attempt.ID] = cancel
	s.mu.Unlock()
	s.workers.Go(func() {
		defer func() { cancel(); s.mu.Lock(); delete(s.stops, attempt.ID); s.mu.Unlock() }()
		err := s.run(ctx, r, attempt, target, remote)
		if err != nil {
			_ = s.patch(r.Candidate, attempt.ID, func(a *mv.Attempt) {
				a.Status = "failed"
				a.Diagnostic = err.Error()
				if errors.Is(err, context.Canceled) {
					a.Status = "stopped"
				}
			})
		}
	})
}

func (s *manualVerification) checkout(ctx context.Context, a mv.Attempt, target ports.RunnerPRTarget, remote string, merged bool) error {
	if a.Worktree != s.worktree(a.ID) {
		return errors.New("核验 worktree 路径不匹配")
	}
	if err := target.Provider.ValidateRemote(remote); err != nil {
		return err
	}
	if err := ordinaryVerificationPath(s.config.RunDir, filepath.Dir(a.Worktree)); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(a.Worktree), 0700); err != nil {
		return err
	}
	if err := ordinaryVerificationPath(s.config.RunDir, filepath.Dir(a.Worktree)); err != nil {
		return err
	}
	privateRef := "refs/steerline/manual-verification/" + a.ID
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = s.git(cleanup, "update-ref", "-d", privateRef)
	}()
	ref := fmt.Sprintf("refs/pull/%d/head", target.Number)
	if a.Source == mv.SourceRemote {
		ref = "refs/heads/" + target.Base
	} else if merged {
		// The provider-confirmed PR position stays fixed after merge.
		ref = a.Head
	}
	if _, err := s.git(ctx, "fetch", "--no-tags", "--no-write-fetch-head", "--no-recurse-submodules", "--", remote, ref+":"+privateRef); err != nil {
		return fmt.Errorf("无法拉取核验提交 %s: %w", a.Head, err)
	}
	head, err := s.git(ctx, "rev-parse", "--verify", privateRef+"^{commit}")
	if err != nil || strings.TrimSpace(string(head)) != a.Head {
		return errors.New("核验来源的 Head 已变化，请重新启动")
	}
	if _, err := s.git(ctx, "-c", "core.hooksPath="+filepath.Join(filepath.Dir(a.Worktree), "disabled-hooks"), "worktree", "add", "--detach", a.Worktree, a.Head); err != nil {
		return err
	}
	return s.verifyWorktree(ctx, a, false)
}

func (s *manualVerification) run(ctx context.Context, r mv.Record, a mv.Attempt, target ports.RunnerPRTarget, remote string) error {
	if err := s.checkout(ctx, a, target, remote, r.Merged); err != nil {
		return err
	}
	launch := r.Definition.Definition.Launch
	cwd := filepath.Join(a.Worktree, filepath.FromSlash(launch.Cwd))
	if err := ordinaryVerificationPath(a.Worktree, cwd); err != nil {
		return err
	}
	if info, err := os.Stat(cwd); err != nil || !info.IsDir() {
		return errors.New("核验工作目录不存在")
	}
	program, err := verificationProgram(a.Worktree, cwd, launch.Program)
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" && (strings.EqualFold(filepath.Ext(program), ".cmd") || strings.EqualFold(filepath.Ext(program), ".bat")) {
		return errors.New("请使用显式解释器和参数启动 Windows 批处理脚本")
	}
	args := append([]string{}, launch.Args...)
	var lease net.Listener
	if strings.Contains(strings.Join(args, "\x00"), mv.Port) {
		lease, a.Port, err = s.reservePort()
		if err != nil {
			return err
		}
		defer lease.Close()
		defer s.releasePort(a.Port)
		for i := range args {
			args[i] = strings.ReplaceAll(args[i], mv.Port, strconv.Itoa(a.Port))
		}
	}
	a.URL = strings.ReplaceAll(launch.URL, mv.Port, strconv.Itoa(a.Port))
	tmp := filepath.Join(filepath.Dir(a.Worktree), "tmp")
	if err := os.Mkdir(tmp, 0700); err != nil {
		return err
	}
	stdout, err := openVerificationLog(filepath.Join(filepath.Dir(a.Worktree), "stdout.log"))
	if err != nil {
		return err
	}
	defer stdout.Close()
	stderr, err := openVerificationLog(filepath.Join(filepath.Dir(a.Worktree), "stderr.log"))
	if err != nil {
		return err
	}
	defer stderr.Close()
	cmd := exec.Command(program, args...)
	cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = cwd, verificationEnvironment(tmp), stdout, stderr
	if err := s.patch(r.Candidate, a.ID, func(v *mv.Attempt) { v.Program = program; v.Args = args; v.Cwd = cwd; v.URL = a.URL; v.Port = a.Port }); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if lease != nil {
		_ = lease.Close()
	}
	owned, err := s.config.ProcessHost.StartManaged(cmd)
	if err != nil {
		return err
	}
	defer owned.Close()
	identity := owned.Identity()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	if err := s.patch(r.Candidate, a.ID, func(v *mv.Attempt) { v.Status = "running"; v.Process = &identity; v.Started = a.URL == "" }); err != nil {
		owned.Kill()
		<-done
		return err
	}
	status, diagnostic, code := s.monitor(ctx, r.Candidate, a, owned, cmd, done)
	if tree, ok := owned.(interface{ WritersAbsent() bool }); !ok || !waitVerificationTree(tree) {
		status, diagnostic = "unknown", "原进程树停止尚未核实"
	}
	return s.patch(r.Candidate, a.ID, func(v *mv.Attempt) { v.Status = status; v.Diagnostic = diagnostic; v.ExitCode = &code })
}

func (s *manualVerification) reservePort() (net.Listener, int, error) {
	s.portMu.Lock()
	defer s.portMu.Unlock()
	for i := 0; i < 8; i++ {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return nil, 0, err
		}
		port := listener.Addr().(*net.TCPAddr).Port
		if !s.ports[port] {
			s.ports[port] = true
			return listener, port, nil
		}
		listener.Close()
	}
	return nil, 0, errors.New("无法分配独立核验端口")
}

func (s *manualVerification) releasePort(port int) {
	s.portMu.Lock()
	delete(s.ports, port)
	s.portMu.Unlock()
}

func waitVerificationTree(tree interface{ WritersAbsent() bool }) bool {
	for i := 0; i < 20; i++ {
		if tree.WritersAbsent() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func verificationTreeAbsent(a mv.Attempt) bool {
	return a.Program == "" || a.Process != nil && processfacts.WorkerTreeAbsent(*a.Process)
}

func (s *manualVerification) monitor(ctx context.Context, candidate string, a mv.Attempt, owned backend.StopHandle, cmd *exec.Cmd, done <-chan error) (string, string, int) {
	var ticks <-chan time.Time
	var deadline <-chan time.Time
	if a.URL != "" {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		ticks = ticker.C
		timer := time.NewTimer(120 * time.Second)
		defer timer.Stop()
		deadline = timer.C
	}
	ready := a.URL == ""
	client := http.Client{Timeout: time.Second, Transport: &http.Transport{DisableKeepAlives: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for {
		select {
		case err := <-done:
			owned.Kill()
			if err != nil || !ready {
				return "failed", "启动程序已退出，请查看日志", cmd.ProcessState.ExitCode()
			}
			return "exited", "", cmd.ProcessState.ExitCode()
		case <-ctx.Done():
			owned.Kill()
			<-done
			return "stopped", "", cmd.ProcessState.ExitCode()
		case <-deadline:
			owned.Kill()
			<-done
			return "failed", "预览未在 120 秒内就绪", cmd.ProcessState.ExitCode()
		case <-ticks:
			request, _ := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
			response, err := client.Do(request)
			if err == nil {
				response.Body.Close()
				if response.StatusCode >= 200 && response.StatusCode < 400 {
					ready = true
					ticks = nil
					deadline = nil
					_ = s.patch(candidate, a.ID, func(v *mv.Attempt) { v.Status = "ready"; v.Started = true })
				}
			}
		}
	}
}

func (s *manualVerification) verifyWorktree(ctx context.Context, a mv.Attempt, clean bool) error {
	if a.Worktree != s.worktree(a.ID) {
		return errors.New("核验 worktree 路径不匹配")
	}
	if err := ordinaryVerificationPath(s.config.RunDir, a.Worktree); err != nil {
		return err
	}
	common, err := s.git(ctx, "-C", a.Worktree, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || canonicalPath(strings.TrimSpace(string(common))) != canonicalPath(s.config.Identity()) {
		return errors.New("核验 worktree 仓库身份不匹配")
	}
	head, err := s.git(ctx, "-C", a.Worktree, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || strings.TrimSpace(string(head)) != a.Head {
		return errors.New("核验 worktree 已切换提交")
	}
	if clean {
		diff, err := s.git(ctx, "-C", a.Worktree, "status", "--porcelain=v1", "--untracked-files=no")
		if err != nil || len(diff) > 0 {
			return errors.New("核验 worktree 的受跟踪文件已变化")
		}
	}
	return nil
}

func ordinaryVerificationPath(root, name string) error {
	resolved, err := pathguard.ResolveProspective(name)
	if err != nil || canonicalPath(name) != canonicalPath(resolved) || !pathguard.Within(root, name) {
		return errors.New("核验路径越界或含重解析点")
	}
	return nil
}

func verificationProgram(root, cwd, program string) (string, error) {
	if strings.HasPrefix(program, "./") {
		name := filepath.Join(cwd, filepath.FromSlash(program[2:]))
		if err := ordinaryVerificationPath(root, name); err != nil {
			return "", err
		}
		return exec.LookPath(name)
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if filepath.IsAbs(dir) && !pathguard.Within(root, dir) {
			if name, err := exec.LookPath(filepath.Join(dir, program)); err == nil {
				return name, nil
			}
		}
	}
	return "", fmt.Errorf("找不到启动程序 %s", program)
}

func verificationEnvironment(tmp string) []string {
	values := []string{"TEMP=" + tmp, "TMP=" + tmp, "TMPDIR=" + tmp}
	for _, key := range []string{"SystemRoot", "SystemDrive", "WINDIR", "COMSPEC", "PATHEXT", "LANG", "LC_ALL"} {
		if value, ok := os.LookupEnv(key); ok {
			values = append(values, key+"="+value)
		}
	}
	var dirs []string
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if filepath.IsAbs(dir) {
			dirs = append(dirs, dir)
		}
	}
	return append(values, "PATH="+strings.Join(dirs, string(os.PathListSeparator)))
}

type verificationLog struct {
	*os.File
	remaining int
}

func openVerificationLog(path string) (*verificationLog, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	return &verificationLog{f, 64 << 10}, nil
}
func (l *verificationLog) Write(p []byte) (int, error) {
	n := min(len(p), l.remaining)
	if n > 0 {
		written, err := l.File.Write(p[:n])
		l.remaining -= written
		if err != nil {
			return written, err
		}
	}
	return len(p), nil
}

func (s *manualVerification) logs(a mv.Attempt) (map[string]string, error) {
	result := map[string]string{}
	for _, name := range []string{"stdout", "stderr"} {
		path := filepath.Join(filepath.Dir(s.worktree(a.ID)), name+".log")
		if err := ordinaryVerificationPath(s.config.RunDir, path); err != nil {
			return nil, err
		}
		if info, err := os.Lstat(path); os.IsNotExist(err) {
			continue
		} else if err != nil || !info.Mode().IsRegular() {
			return nil, errors.New("无法读取核验日志")
		}
		if err := pathguard.RejectHardLinkedFile(path); err != nil {
			return nil, err
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(file, 64<<10))
		file.Close()
		if err != nil {
			return nil, err
		}
		result[name] = string(body)
	}
	return result, nil
}
