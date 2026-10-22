package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	backend "github.com/erranliu/SteerLine/internal/execution/backend"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

type observationStore interface {
	MaterialObservation(context.Context, string) (pm.Observation, error)
	SaveMaterialObservation(context.Context, string, pm.Observation) error
}
type cachedBaselineCommit struct {
	value baselineCommit
	at    time.Time
}
type baselineCommit struct {
	SHA          string   `json:"sha"`
	Title        string   `json:"title"`
	Author       string   `json:"author"`
	Time         string   `json:"time"`
	URL          string   `json:"url"`
	PRs          []string `json:"prs"`
	PRDiagnostic string   `json:"pr_diagnostic,omitempty"`
}
type localBaseline struct {
	Commit    string `json:"commit"`
	Branch    string `json:"branch"`
	Status    string `json:"status"`
	Ahead     int    `json:"ahead"`
	Behind    int    `json:"behind"`
	CanUpdate bool   `json:"can_update"`
	Reason    string `json:"reason,omitempty"`
}

func (m *projectMaintenance) commitInfo(ctx context.Context, sha string, pr bool) baselineCommit {
	m.mu.Lock()
	cached, ok := m.commitCache[sha]
	m.mu.Unlock()
	if ok && time.Since(cached.at) < time.Minute {
		return cached.value
	}
	info := baselineCommit{SHA: sha, PRs: []string{}}
	if sha == "" {
		return info
	}
	raw, e := m.git(ctx, "show", "-s", "--format=%H%x00%cI%x00%an%x00%s", sha, "--")
	if e != nil {
		info.Title = e.Error()
		return info
	}
	fields := strings.SplitN(strings.TrimSpace(string(raw)), "\x00", 4)
	if len(fields) == 4 {
		info.SHA, info.Time, info.Author, info.Title = fields[0], fields[1], fields[2], fields[3]
	}
	repo := strings.TrimSuffix(m.config.PR.Repository, ".git")
	host := strings.TrimSuffix(m.config.PR.BaseURL, "/")
	if (host == "" || host == "https://api.github.com") && m.config.PR.Kind == "github" {
		host = "https://github.com"
	}
	if host != "" && repo != "" {
		info.URL = strings.TrimSuffix(host, "/api/v3") + "/" + repo + "/commit/" + sha
	}
	if pr {
		info.PRs, info.PRDiagnostic = m.commitPullRequests(ctx, sha)
	}

	if pr {
		m.mu.Lock()
		if m.commitCache == nil {
			m.commitCache = map[string]cachedBaselineCommit{}
		}
		m.commitCache[sha] = cachedBaselineCommit{info, time.Now()}
		m.mu.Unlock()
	}
	return info
}
func (m *projectMaintenance) localBaseline(ctx context.Context) localBaseline {
	result := localBaseline{Status: "unknown"}
	head, e := m.git(ctx, "rev-parse", "--verify", "HEAD^{commit}")
	if e != nil {
		result.Reason = e.Error()
		return result
	}
	result.Commit = strings.TrimSpace(string(head))
	branch, e := m.git(ctx, "symbolic-ref", "--quiet", "--short", "HEAD")
	if e != nil {
		result.Reason = "当前为游离HEAD，请人工切回目标分支 " + m.config.Branch
		return result
	}
	result.Branch = strings.TrimSpace(string(branch))
	o := m.current()
	if o.Commit == "" || o.Diagnostic != "" {
		result.Reason = "远端状态待核实，请检查远端"
		return result
	}
	raw, e := m.git(ctx, "rev-list", "--left-right", "--count", result.Commit+"..."+o.Commit, "--")
	if e != nil {
		result.Reason = e.Error()
		return result
	}
	if _, e = fmt.Sscanf(string(raw), "%d %d", &result.Ahead, &result.Behind); e != nil {
		result.Reason = e.Error()
		return result
	}
	switch {
	case result.Ahead == 0 && result.Behind == 0:
		result.Status = "synced"
	case result.Ahead > 0 && result.Behind > 0:
		result.Status = "diverged"
	case result.Ahead > 0:
		result.Status = "ahead"
	default:
		result.Status = "behind"
	}
	if result.Branch != m.config.Branch {
		result.Reason = "当前分支 " + result.Branch + " 与目标 " + m.config.Branch + " 不同，请人工处理"
		return result
	}
	if _, e = m.workspace(ctx); e != nil {
		result.Reason = "Git操作尚未结束：" + e.Error()
		return result
	}
	dirty, e := m.git(ctx, "status", "--porcelain", "--untracked-files=all")
	if e != nil {
		result.Reason = e.Error()
		return result
	}
	if len(dirty) > 0 {
		result.Reason = "工作区有未提交或未跟踪文件，请先人工保存：" + string(dirty)
		return result
	}
	if o.Rewritten {
		result.Reason = "远端历史已改写，请人工核对Git历史"
		return result
	}
	if result.Status == "behind" {
		result.CanUpdate = true
	} else if result.Status == "ahead" || result.Status == "diverged" {
		result.Reason = "本地存在独有提交，请人工处理后重新检查"
	}
	return result
}
func (m *projectMaintenance) baselineView(ctx context.Context) (any, error) {
	o := m.current()
	store, ok := m.store.(materialStore)
	if !ok {
		return nil, errors.New("material store unavailable")
	}
	graph, e := m.store.TaskGraphSnapshot(ctx, m.config.Identity())
	if e != nil {
		return nil, e
	}
	materials := []pm.Material{}
	for _, task := range graph.Tasks {
		value, e := store.Material(ctx, m.config.Identity(), task.TaskID)
		if e == nil {
			materials = append(materials, value)
		}
	}
	type profile struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	profiles := []profile{}
	for _, p := range m.config.RunnerExecution.Profiles {
		if _, e := generationProfile(p); e == nil {
			profiles = append(profiles, profile{p.ID, p.Name})
		}
	}
	var progress pm.GenerationProgress
	if batchStore, ok := m.store.(generationBatchStore); ok {
		batch, err := batchStore.GenerationBatch(ctx, m.config.Identity())
		if err != nil && !errors.Is(err, storageports.ErrNotFound) {
			return nil, err
		}
		progress = m.batchProgress(ctx, batch)
	}
	local := m.localBaseline(ctx)
	remote := m.commitInfo(ctx, o.Commit, true)
	localInfo := remote
	if local.Commit != remote.SHA {
		localInfo = m.commitInfo(ctx, local.Commit, false)
	}
	return struct {
		Observation pm.Observation        `json:"observation"`
		LocalInfo   baselineCommit        `json:"local_info"`
		RemoteName  string                `json:"remote_name"`
		Remote      baselineCommit        `json:"remote"`
		Local       localBaseline         `json:"local"`
		Tasks       []pm.Material         `json:"tasks"`
		Profiles    []profile             `json:"profiles"`
		Profile     string                `json:"profile"`
		Progress    pm.GenerationProgress `json:"generation_progress"`
	}{o, localInfo, m.config.Remote + "/" + m.config.Branch, remote, local, materials, profiles, store.MaterialProfile(ctx, m.config.Identity()), progress}, nil
}
func generationProfile(p RunnerProfile) (bool, error) {
	if p.Execution.Adapter != "codex-app-server" || p.Execution.Target.Kind != "local" {
		return false, errors.New("需要本机Codex方案")
	}
	return true, nil
}
func (m *projectMaintenance) commitLog(ctx context.Context, page, scope string) (any, error) {
	n, e := strconv.Atoi(page)
	if e != nil || n < 0 || n > 10000 {
		return nil, errors.New("invalid log page")
	}
	o := m.current()
	if o.Commit == "" {
		return nil, errors.New("请先检查远端")
	}
	target := o.Commit
	switch scope {
	case "", "history":
	case "behind", "ahead":
		local := m.localBaseline(ctx)
		if local.Commit == "" {
			return nil, errors.New("本地版本未知")
		}
		target = local.Commit + ".." + o.Commit
		if scope == "ahead" {
			target = o.Commit + ".." + local.Commit
		}
	default:
		return nil, errors.New("invalid log scope")
	}
	raw, e := m.git(ctx, "log", "--format=%H", "--max-count=21", "--skip="+strconv.Itoa(n*20), target, "--")
	if e != nil {
		return nil, e
	}
	hashes := strings.Fields(string(raw))
	more := len(hashes) > 20
	if more {
		hashes = hashes[:20]
	}
	commits := []baselineCommit{}
	for _, sha := range hashes {
		commits = append(commits, m.commitInfo(ctx, sha, false))
	}
	return struct {
		Commits []baselineCommit `json:"commits"`
		More    bool             `json:"more"`
	}{commits, more}, nil
}

type updateFact struct {
	Before     string `json:"before"`
	Target     string `json:"target"`
	Status     string `json:"status"`
	Diagnostic string `json:"diagnostic,omitempty"`
}

// Only classifies the original durable intent; it never fetches or writes Git.
func (m *projectMaintenance) observeUpdate(ctx context.Context, id string) (updateFact, error) {
	raw, e := m.store.MaintenanceReceipt(ctx, m.config.Identity(), id)
	if e != nil {
		if !errors.Is(e, storageports.ErrNotFound) {
			return updateFact{}, e
		}
		return updateFact{Status: "rejected", Diagnostic: "原操作尚未保存Git写入意图，未开始更新；请重新检查后重试"}, nil
	}
	var fact updateFact
	if e = json.Unmarshal(raw, &fact); e != nil {
		return fact, e
	}
	head, e := m.git(ctx, "rev-parse", "--verify", "HEAD^{commit}")
	if e == nil && strings.TrimSpace(string(head)) == fact.Target {
		fact.Status = "applied"
		fact.Diagnostic = ""
	} else if fact.Status == "pending" {
		fact.Status = "outcome_unknown"
		fact.Diagnostic = "原Git更新结果待核实，请人工核对；不会自动补写"
	}
	return fact, nil
}

type maintenanceOperations interface {
	SaveMaintenanceOperation(context.Context, string, string, []byte) error
}

func (m *projectMaintenance) updateLocal(ctx context.Context, id string) (any, error) {
	persistence, ok := m.store.(maintenanceOperations)
	if !ok {
		return nil, errors.New("operation store unavailable")
	}
	save := func(f updateFact) error {
		raw, _ := json.Marshal(f)
		return persistence.SaveMaintenanceOperation(context.WithoutCancel(ctx), m.config.Identity(), id, raw)
	}
	if raw, e := m.store.MaintenanceReceipt(ctx, m.config.Identity(), id); e == nil {
		var f updateFact
		if e = json.Unmarshal(raw, &f); e != nil {
			return nil, e
		}
		head, e := m.git(ctx, "rev-parse", "--verify", "HEAD^{commit}")
		if e == nil && strings.TrimSpace(string(head)) == f.Target {
			f.Status = "applied"
			f.Diagnostic = ""
		} else if f.Status == "pending" {
			f.Status = "outcome_unknown"
			f.Diagnostic = "原快进结果未核实，请人工核对；不会重新更新到其他目标"
		}
		return f, save(f)
	}
	if _, e := m.check(ctx); e != nil {
		return nil, e
	}
	local := m.localBaseline(ctx)
	if !local.CanUpdate {
		return nil, fmt.Errorf("无法自动更新：%s", local.Reason)
	}
	lockPath, e := m.git(ctx, "rev-parse", "--git-path", "steerline-update.lock")
	if e != nil {
		return nil, e
	}
	path := strings.TrimSpace(string(lockPath))
	if !filepath.IsAbs(path) {
		path = filepath.Join(m.config.Repository, path)
	}
	lock, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return nil, errors.New("Git更新锁被占用，请人工核对后重试")
	}
	defer func() { lock.Close(); os.Remove(path) }()
	fresh := m.localBaseline(ctx)
	if !fresh.CanUpdate || fresh.Commit != local.Commit {
		return nil, errors.New("本地状态已变化，请重新检查")
	}
	fact := updateFact{Before: local.Commit, Target: m.current().Commit, Status: "pending"}
	if e = save(fact); e != nil {
		return nil, e
	}
	_, callErr := m.git(ctx, "-c", "core.hooksPath=", "merge", "--ff-only", "--no-edit", "--no-overwrite-ignore", fact.Target)
	head, readErr := m.git(context.WithoutCancel(ctx), "rev-parse", "--verify", "HEAD^{commit}")
	if readErr == nil && strings.TrimSpace(string(head)) == fact.Target {
		fact.Status = "applied"
	} else {
		fact.Status = "outcome_unknown"
		fact.Diagnostic = fmt.Sprint(callErr, "；请人工核对主仓库状态")
	}
	return fact, save(fact)
}

// Gitea documents this read-only association at /commits/{sha}/pull:
// https://docs.gitea.com/api/1.25/operations/repo-get-commit-pull-request/
func (m *projectMaintenance) commitPullRequests(ctx context.Context, sha string) ([]string, string) {
	urls := []string{}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	config := m.config.PR
	host := strings.TrimRight(config.BaseURL, "/")
	if config.Kind == "command" && host == "https://github.com" {
		env := map[string]string{}
		if config.TokenEnv != "" {
			if token := os.Getenv(config.TokenEnv); token != "" {
				env["GH_TOKEN"] = token
			}
		}
		raw, e := (commandRunner{Dir: m.config.Repository, Timeout: 5 * time.Second, ProcessHost: m.config.ProcessHost, Env: env}).Output(ctx, backend.Command{Executable: "gh"}, "api", "--hostname", "github.com", "repos/"+config.Repository+"/commits/"+sha+"/pulls", "--jq", ".[].html_url")
		if e != nil {
			return urls, "关联PR查询失败；Git版本信息仍可使用"
		}
		for _, url := range strings.Fields(string(raw)) {
			if _, e := config.PRNumber(url); e != nil {
				return []string{}, "关联PR响应身份无效"
			}
			urls = append(urls, url)
		}
		return urls, ""
	}
	if config.Kind != "gitea" {
		return urls, "当前provider暂不支持按提交查询关联PR"
	}
	request, e := http.NewRequestWithContext(ctx, http.MethodGet, host+"/api/v1/repos/"+config.Repository+"/commits/"+sha+"/pull", nil)
	if e != nil {
		return urls, "关联PR查询配置无效"
	}
	if config.TokenEnv != "" {
		token := os.Getenv(config.TokenEnv)
		if token == "" {
			return urls, "关联PR认证未配置；请核对 " + config.TokenEnv
		}
		request.Header.Set("Authorization", "token "+token)
	}
	client := http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, e := client.Do(request)
	if e != nil {
		return urls, "关联PR查询失败；Git版本信息仍可使用"
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		// Private/unauthorized repositories can also return 404. Verify access
		// before telling the user that the commit has no associated PR.
		probe, e := http.NewRequestWithContext(ctx, http.MethodGet, host+"/api/v1/repos/"+config.Repository, nil)
		if e != nil {
			return urls, "关联PR查询配置无效"
		}
		probe.Header = request.Header.Clone()
		access, e := client.Do(probe)
		if e != nil {
			return urls, "关联PR查询失败；仓库访问权限待核实"
		}
		access.Body.Close()
		if access.StatusCode != http.StatusOK {
			return urls, "关联PR查询失败；请核对仓库访问权限"
		}
		return urls, ""
	}
	if response.StatusCode != http.StatusOK {
		return urls, fmt.Sprintf("关联PR查询失败（HTTP %d）；Git版本信息仍可使用", response.StatusCode)
	}
	var result struct {
		URL string `json:"html_url"`
	}
	if e = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); e != nil {
		return urls, "关联PR响应无效"
	}
	if _, e = config.PRNumber(result.URL); e != nil {
		return urls, "关联PR响应身份无效"
	}
	return append(urls, result.URL), ""
}
