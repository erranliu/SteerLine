package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	backend "github.com/erranliu/SteerLine/internal/execution/backend"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type roadmapPR struct {
	URL         string `json:"html_url"`
	Number      int    `json:"number"`
	Merged      bool   `json:"merged"`
	MergeCommit string `json:"merge_commit_sha"`
	State       string `json:"state"`
	Head        struct {
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

func (p *roadmapPort) provider(ctx context.Context, method, endpoint string, input any, out any) error {
	config := p.config.PR
	if !strings.HasPrefix(endpoint, "repos/"+config.Repository+"/") {
		return errors.New("provider target differs")
	}
	var body []byte
	var e error
	if input != nil {
		body, e = json.Marshal(input)
		if e != nil {
			return e
		}
	}
	if config.Kind == "command" && strings.TrimRight(config.BaseURL, "/") == "https://github.com" {
		args := []string{"api", "--hostname", "github.com", "--method", method, endpoint}
		if len(body) > 0 {
			file, e := os.CreateTemp(p.config.RunDir, "roadmap-provider-*.json")
			if e != nil {
				return e
			}
			name := file.Name()
			defer os.Remove(name)
			if _, e = file.Write(body); e != nil {
				file.Close()
				return e
			}
			if e = file.Close(); e != nil {
				return e
			}
			args = append(args, "--input", name)
		}
		env := map[string]string{}
		if config.TokenEnv != "" {
			if token := os.Getenv(config.TokenEnv); token != "" {
				env["GH_TOKEN"] = token
			}
		}
		raw, e := (commandRunner{Dir: p.config.Repository, Timeout: 30 * time.Second, ProcessHost: p.config.ProcessHost, Env: env}).Output(ctx, backend.Command{Executable: "gh"}, args...)
		if e != nil {
			return errors.New("文档 PR provider 操作失败，请查询原 PR 核实")
		}
		if out != nil {
			return json.Unmarshal(raw, out)
		}
		return nil
	}
	if config.Kind != "gitea" {
		return errors.New("当前 provider 不支持文档 PR 发布")
	}
	request, e := http.NewRequestWithContext(ctx, method, strings.TrimRight(config.BaseURL, "/")+"/api/v1/"+endpoint, bytes.NewReader(body))
	if e != nil {
		return e
	}
	request.Header.Set("Content-Type", "application/json")
	if config.TokenEnv != "" {
		token := os.Getenv(config.TokenEnv)
		if token == "" {
			return errors.New("文档发布认证未配置")
		}
		request.Header.Set("Authorization", "token "+token)
	}
	client := http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, e := client.Do(request)
	if e != nil {
		return errors.New("文档 PR 请求结果待核实")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("文档 PR provider 返回 HTTP %d，请查询原操作", response.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(response.Body, roadmap.MaxBytes+1)).Decode(out)
	}
	return nil
}
func (p *roadmapPort) prFact(ctx context.Context, prURL string) (roadmapPR, error) {
	var fact roadmapPR
	number, e := p.config.PR.PRNumber(prURL)
	if e != nil {
		return fact, e
	}
	e = p.provider(ctx, "GET", "repos/"+p.config.PR.Repository+"/pulls/"+strconv.Itoa(number), nil, &fact)
	if e != nil {
		return fact, e
	}
	if fact.Number != number || fact.Head.Repo.FullName != p.config.PR.Repository {
		return fact, errors.New("文档 PR 仓库/编号身份不同")
	}
	if _, e = p.config.PR.PRNumber(fact.URL); e != nil {
		return fact, e
	}
	return fact, nil
}
func (p *roadmapPort) findPR(ctx context.Context, branch string) (*roadmapPR, error) {
	var match *roadmapPR
	for page := 1; page <= 100; page++ {
		var list []roadmapPR
		query := "?state=all&page=" + strconv.Itoa(page) + "&limit=50&per_page=50"
		if e := p.provider(ctx, "GET", "repos/"+p.config.PR.Repository+"/pulls"+query, nil, &list); e != nil {
			return nil, e
		}
		for i := range list {
			f := list[i]
			if f.Head.Ref == branch && f.Base.Ref == p.config.Branch {
				if match != nil {
					return nil, errors.New("存在多个匹配的文档 PR，请人工核对")
				}
				copy := f
				match = &copy
			}
		}
		if len(list) < 50 {
			return match, nil
		}
	}
	return nil, errors.New("PR 列表超过查询上限，不能推定原 PR 不存在")
}
func (p *roadmapPort) publicationFiles(ctx context.Context, r roadmap.Record) (map[string]string, error) {
	if e := roadmap.ValidateDocuments(r); e != nil {
		return nil, e
	}
	out := map[string]string{}
	for _, d := range r.Documents {
		out[d.Spec.Path] = d.Spec.Body
		out[d.Plan.Path] = d.Plan.Body
		for _, a := range d.Acceptance {
			out[a.Path] = a.Body
		}
	}
	raw, e := json.MarshalIndent(struct {
		Roadmap   string              `json:"roadmap_id"`
		Source    roadmap.Source      `json:"source"`
		Analysis  *roadmap.Analysis   `json:"analysis"`
		Documents []roadmap.Documents `json:"documents"`
	}{r.Source.ID, r.Source, r.Analysis, r.Documents}, "", "  ")
	if e != nil {
		return nil, e
	}
	out["SteerlineFlow/Roadmap/generated/"+r.Source.ID+".json"] = string(raw) + "\n"
	old, e := p.config.runtimeMaintenance.read(ctx, r.CodeCommit, "SteerlineFlow/Roadmap/execution-map.md")
	if e != nil && !errors.Is(e, os.ErrNotExist) {
		return nil, e
	}
	var addition strings.Builder
	fmt.Fprintf(&addition, "\n\n## Roadmap %s\n\n内部代号关联，正式编号和执行状态由 SteerLine Task 权威维护。\n\n", r.Source.ID)
	for _, n := range r.Analysis.Tasks {
		fmt.Fprintf(&addition, "- %s / %s：%s；批内前置 %s；正式前置 %s。\n", r.Source.ID, n.Key, n.Title, strings.Join(n.Proposed.Local, ", "), strings.Join(n.Proposed.Formal, ", "))
	}
	out["SteerlineFlow/Roadmap/execution-map.md"] = string(old) + addition.String()
	return out, nil
}
func writeRoadmapFile(root, relative, body string) error {
	if strings.ContainsAny(relative, "\\:\x00") || filepath.ToSlash(filepath.Clean(relative)) != relative || strings.HasPrefix(relative, "../") || filepath.IsAbs(relative) {
		return errors.New("发布文件路径越界")
	}
	cursor := root
	parts := strings.Split(relative, "/")
	for _, part := range parts[:len(parts)-1] {
		cursor = filepath.Join(cursor, part)
		info, e := os.Lstat(cursor)
		if os.IsNotExist(e) {
			if e = os.Mkdir(cursor, 0700); e != nil {
				return e
			}
		} else if e != nil {
			return e
		} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("发布目录不是普通目录")
		}
	}
	file := filepath.Join(root, filepath.FromSlash(relative))
	if info, e := os.Lstat(file); e == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return errors.New("发布目标不是普通文件")
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	resolved, e := filepath.EvalSymlinks(filepath.Dir(file))
	if e != nil {
		return e
	}
	if canonicalPath(resolved) != canonicalPath(filepath.Dir(file)) {
		return errors.New("发布路径含有重定向")
	}
	return os.WriteFile(file, []byte(body), 0600)
}
func (p *roadmapPort) preparePublication(ctx context.Context, r *roadmap.Record) error {
	pub := r.Publication
	m := p.config.runtimeMaintenance
	files, e := p.publicationFiles(ctx, *r)
	if e != nil {
		return e
	}
	directory := filepath.Join(p.config.RunDir, "roadmap-publications", r.ID)
	if _, e = os.Stat(directory); os.IsNotExist(e) {
		if e = os.MkdirAll(filepath.Dir(directory), 0700); e != nil {
			return e
		}
		if _, e = m.git(ctx, "worktree", "add", "-b", pub.Branch, directory, pub.Base); e != nil {
			return e
		}
	} else if e != nil {
		return e
	}
	git := func(args ...string) ([]byte, error) { return m.git(ctx, append([]string{"-C", directory}, args...)...) }
	common, e := git("rev-parse", "--path-format=absolute", "--git-common-dir")
	if e != nil || canonicalPath(strings.TrimSpace(string(common))) != canonicalPath(p.config.Identity()) {
		return errors.New("文档 worktree 的仓库身份不同")
	}
	branch, e := git("symbolic-ref", "--short", "HEAD")
	if e != nil || strings.TrimSpace(string(branch)) != pub.Branch {
		return errors.New("文档分支绑定不同")
	}
	keys := []string{}
	for name := range files {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		if name != "SteerlineFlow/Roadmap/execution-map.md" {
			if old, e := m.read(ctx, pub.Base, name); e == nil {
				if !ownedRoadmapMetadata(name, old, *r) {
					return errors.New("拒绝覆盖已存在的无关文件：" + name)
				}
			} else if !errors.Is(e, os.ErrNotExist) {
				return e
			}
		}
	}
	// Refuse unrelated modifications in the isolated worktree, including untracked files.
	for _, args := range [][]string{{"diff", "--name-only", "-z", "HEAD"}, {"ls-files", "--others", "--exclude-standard", "-z"}} {
		raw, e := git(args...)
		if e != nil {
			return e
		}
		for _, name := range strings.Split(string(raw), "\x00") {
			if name != "" {
				if _, ok := files[name]; !ok {
					return errors.New("文档 worktree 含无关修改")
				}
			}
		}
	}
	for _, name := range keys {
		if e = writeRoadmapFile(directory, name, files[name]); e != nil {
			return e
		}
	}
	if _, e = git(append([]string{"add", "--"}, keys...)...); e != nil {
		return e
	}
	staged, e := git("diff", "--cached", "--name-only", "-z")
	if e != nil {
		return e
	}
	if len(staged) > 0 {
		if _, e = git("commit", "-m", "Publish roadmap documents "+r.Source.ID); e != nil {
			return e
		}
	}
	head, e := git("rev-parse", "HEAD^{commit}")
	if e != nil {
		return e
	}
	pub.Commit = strings.TrimSpace(string(head))
	if !taskmodel.ValidCommit(pub.Commit) || pub.Commit == pub.Base {
		return errors.New("文档提交未形成")
	}
	for name, body := range files {
		got, e := m.read(ctx, pub.Commit, name)
		if e != nil || string(got) != body {
			return errors.New("文档提交内容未核实：" + name)
		}
	}
	return nil
}

// Later batches may replace this source's generated index, never its original
// Markdown or another source's documents. The full before/after is reviewed.
func ownedRoadmapMetadata(name string, body []byte, r roadmap.Record) bool {
	if name != "SteerlineFlow/Roadmap/generated/"+r.Source.ID+".json" {
		return false
	}
	var old struct {
		Roadmap string         `json:"roadmap_id"`
		Source  roadmap.Source `json:"source"`
	}
	return json.Unmarshal(body, &old) == nil && old.Roadmap == r.Source.ID && old.Source.ID == r.Source.ID && old.Source.Path == r.Source.Path
}
func (p *roadmapPort) publish(ctx context.Context, r roadmap.Record, request, review string, merge bool) (roadmap.Record, error) {
	p.publicationMu.Lock()
	defer p.publicationMu.Unlock()
	if r.State != "documents-review" && r.Publication == nil {
		return r, errors.New("请先取得并审阅完整正式文档")
	}
	if e := roadmap.ValidateDocuments(r); e != nil {
		return r, e
	}
	if review != roadmap.ReviewDigest(r) {
		return r, errors.New("审阅内容已变化，请重新审阅")
	}
	if r.Publication == nil {
		rows, err := p.store.Roadmaps(ctx, p.config.Identity())
		if err != nil {
			return r, err
		}
		for _, other := range rows {
			if other.ID != r.ID && other.Source.ID == r.Source.ID && other.Publication != nil && other.State != "admitted" {
				return r, errors.New("同一来源已有在途文档发布，请先核实原请求")
			}
		}
	}
	if _, e := p.config.TaskRepositoryTarget(ctx); e != nil {
		return r, e
	}
	if p.config.PR.Kind != "gitea" && !(p.config.PR.Kind == "command" && strings.TrimRight(p.config.PR.BaseURL, "/") == "https://github.com") {
		return r, errors.New("文档发布 provider 能力不可用")
	}
	var e error
	if r.Publication != nil {
		if r.Publication.RequestID != request || r.Publication.ReviewDigest != review || r.Publication.Merge != merge {
			return r, errors.New("请查询原文档发布请求，不得替换原授权/输入")
		}
		return p.observePublication(ctx, r)
	}
	source, err := p.source(ctx, r.Source.Path)
	if err != nil {
		return r, err
	}
	graph, err := p.graph(ctx)
	if err != nil {
		return r, err
	}
	baseline, err := p.config.runtimeMaintenance.check(ctx)
	if err != nil {
		return r, err
	}
	if source.Digest != r.Source.Digest || roadmap.GraphDigest(graph) != r.GraphDigest || baseline.Commit != r.CodeCommit {
		return r, errors.New("来源、图材料或代码基线已变化，请重新核对生成文档")
	}
	r.Publication = &roadmap.Publication{RequestID: request, ReviewDigest: review, Branch: "codex/roadmap-" + r.ID, Base: r.CodeCommit, Merge: merge, State: "preparing"}
	r.State = "publishing"
	r, e = p.serviceSave(ctx, r)
	if e != nil {
		return r, e
	}
	return p.continuePublication(ctx, r)
}

// Resume only proven unsubmitted phases. Unknown remote effects are observed,
// never submitted again. All input and merge authorization stay on the original intent.
func (p *roadmapPort) resumePublication(ctx context.Context, r roadmap.Record) (roadmap.Record, error) {
	p.publicationMu.Lock()
	defer p.publicationMu.Unlock()
	var err error
	r, err = p.store.Roadmap(ctx, p.config.Identity(), r.ID)
	if err != nil {
		return r, err
	}
	if r.Publication == nil {
		return r, errors.New("没有原文档发布请求")
	}
	if e := roadmap.ValidateDocuments(r); e != nil {
		return r, e
	}
	if r.Publication.ReviewDigest != roadmap.ReviewDigest(r) {
		return r, errors.New("原发布审阅摘要不匹配")
	}
	return p.continuePublication(ctx, r)
}

func (p *roadmapPort) continuePublication(ctx context.Context, r roadmap.Record) (roadmap.Record, error) {
	var e error
	if r.Publication.State == "preparing" {
		if e = p.preparePublication(ctx, &r); e != nil {
			r.Publication.Diagnostic = e.Error()
			r.Diagnostic = e.Error()
			r, _ = p.serviceSave(context.WithoutCancel(ctx), r)
			return r, e
		}
		r.Publication.State = "push_unknown"
		r, e = p.serviceSave(ctx, r)
		if e != nil {
			return r, e
		}
		_, e = p.config.runtimeMaintenance.git(ctx, "push", "--force-with-lease=refs/heads/"+r.Publication.Branch+":", p.config.Remote, r.Publication.Commit+":refs/heads/"+r.Publication.Branch)
		if e != nil {
			return r, errors.New("原文档推送结果待核实")
		}
		r.Publication.State = "pushed"
		r, e = p.serviceSave(ctx, r)
		if e != nil {
			return r, e
		}
	}
	if r.Publication.State == "push_unknown" {
		remote, err := p.config.runtimeMaintenance.git(ctx, "ls-remote", "--refs", p.config.Remote, "refs/heads/"+r.Publication.Branch)
		if err != nil || strings.TrimSpace(string(remote)) != r.Publication.Commit+"\trefs/heads/"+r.Publication.Branch {
			return r, errors.New("原推送尚未核实，不重复推送")
		}
		r.Publication.State = "pushed"
		r, e = p.serviceSave(ctx, r)
		if e != nil {
			return r, e
		}
	}
	if r.Publication.State != "pushed" && r.Publication.State != "pr" {
		return p.observePublication(ctx, r)
	}
	if r.Publication.State == "pushed" {
		existing, e := p.findPR(ctx, r.Publication.Branch)
		if e != nil {
			return r, e
		}
		if existing == nil {
			r.Publication.State = "pr_unknown"
			r, e = p.serviceSave(ctx, r)
			if e != nil {
				return r, e
			}
			var fact roadmapPR
			e = p.provider(ctx, "POST", "repos/"+p.config.PR.Repository+"/pulls", map[string]any{"title": "Roadmap 文档：" + r.Source.Path, "head": r.Publication.Branch, "base": p.config.Branch, "body": "固定 Roadmap 内部身份、Spec / Plan 和依赖。正式编号仅在入 DAG 时分配。"}, &fact)
			if e != nil {
				return r, e
			}
			existing = &fact
		}
		r.Publication.URL = existing.URL
		r.Publication.State = "pr"
		r, e = p.serviceSave(ctx, r)
		if e != nil {
			return r, e
		}
	}
	if r.Publication.Merge {
		fact, e := p.prFact(ctx, r.Publication.URL)
		if e != nil {
			return r, e
		}
		if fact.Head.SHA != r.Publication.Commit || fact.Base.Ref != p.config.Branch || fact.Head.Ref != r.Publication.Branch {
			return r, errors.New("待合并 PR 的 head/base 与原文档发布不同")
		}
		if !fact.Merged {
			r.Publication.State = "merge_unknown"
			r, e = p.serviceSave(ctx, r)
			if e != nil {
				return r, e
			}
			body := map[string]any{"do": "merge", "head_commit_id": r.Publication.Commit}
			method := "POST"
			if p.config.PR.Kind == "command" {
				method = "PUT"
				body = map[string]any{"merge_method": "merge", "sha": r.Publication.Commit}
			}
			if e = p.provider(ctx, method, "repos/"+p.config.PR.Repository+"/pulls/"+strconv.Itoa(fact.Number)+"/merge", body, nil); e != nil {
				return r, e
			}
		}
	}
	return p.observePublication(ctx, r)
}
func (p *roadmapPort) observePublication(ctx context.Context, r roadmap.Record) (roadmap.Record, error) {
	if r.Publication == nil {
		return r, errors.New("没有原文档发布请求")
	}
	if r.Publication.URL == "" {
		fact, e := p.findPR(ctx, r.Publication.Branch)
		if e != nil {
			return r, e
		}
		if fact == nil {
			return r, errors.New("原文档发布结果仍未知；不重复推送/创建 PR")
		}
		r.Publication.URL = fact.URL
	}
	fact, e := p.prFact(ctx, r.Publication.URL)
	if e != nil {
		return r, e
	}
	if fact.Head.Ref != r.Publication.Branch || fact.Head.SHA != r.Publication.Commit || fact.Base.Ref != p.config.Branch {
		return r, errors.New("文档 PR 身份不同")
	}
	if !fact.Merged {
		r.State = "awaiting-merge"
		if r.Publication.State != "merge_unknown" {
			r.Publication.State = "pr"
		}
		return p.serviceSave(ctx, r)
	}
	if e = p.verifyMerged(ctx, &r); e != nil {
		return r, e
	}
	return p.serviceSave(ctx, r)
}
