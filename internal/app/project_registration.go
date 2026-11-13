package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	envmodel "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	backend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	"github.com/erranliu/SteerLine/internal/execution/workspace"
	"github.com/erranliu/SteerLine/internal/pathguard"
	api "github.com/erranliu/SteerLine/internal/product/api"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/product/httpfs"
	"github.com/erranliu/SteerLine/internal/projectconnection"
	"github.com/erranliu/SteerLine/internal/prompts"
	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
)

var projectRegistrationName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

func registrationError(status int, code, reason string) error {
	return &httpclient.ClientError{Status: status, Code: code, Reason: reason}
}

func (d *ApplicationDirectory) ProjectRegistrationPrompt() (contract.ProjectRegistrationPrompt, error) {
	initialization, err := prompts.ProjectInitialization()
	if err != nil {
		return contract.ProjectRegistrationPrompt{}, err
	}
	body, err := prompts.ProjectRegistration(d.home)
	return contract.ProjectRegistrationPrompt{InitializationPrompt: initialization, Prompt: body}, err
}

func (d *ApplicationDirectory) RegisterProject(ctx context.Context, input contract.ProjectRegistration) (contract.ProjectRegistrationResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	result, ref, data, err := d.checkProjectRegistration(ctx, input)
	if err != nil {
		return result, err
	}
	confirm := input.Confirmation != ""
	if confirm && input.Confirmation != result.Confirmation {
		return result, registrationError(409, "registration_changed", "仓库、目标分支或配置已变化，请重新核验")
	}
	if !confirm {
		_, err := d.checkRegistrationConflicts(ctx, ref, data, false)
		return result, err
	}
	err = httpfs.WithConfigurationLock(d.home, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.ctx.Err() != nil {
			return registrationError(409, "application_stopping", "安装实例正在停止，请在重启后重新核验")
		}
		saved, err := d.checkRegistrationConflicts(ctx, ref, data, true)
		if err != nil || saved {
			return err
		}
		d.mu.RLock()
		defer d.mu.RUnlock()
		if d.stopping {
			return registrationError(409, "application_stopping", "安装实例正在停止，请在重启后重新核验")
		}
		return d.saveProjectRegistration(ref.ConfigPath, data)
	})
	if err == nil {
		result.Saved = true
		result.Confirmation = ""
	}
	return result, err
}

func (d *ApplicationDirectory) checkProjectRegistration(ctx context.Context, in contract.ProjectRegistration) (contract.ProjectRegistrationResult, api.ProjectRef, []byte, error) {
	var result contract.ProjectRegistrationResult
	var ref api.ProjectRef
	invalid := func(reason string) (contract.ProjectRegistrationResult, api.ProjectRef, []byte, error) {
		return result, ref, nil, registrationError(400, "registration_invalid", reason)
	}
	if !projectRegistrationName.MatchString(in.Name) || !filepath.IsLocal(in.Name+".json") || strings.HasSuffix(in.Name, ".") ||
		!filepath.IsAbs(in.Repository) || in.Remote == "" || in.Branch == "" || strings.HasPrefix(in.Branch, "-") {
		return invalid("填写项目名称、服务机器上的绝对仓库路径、Git remote 和目标分支")
	}
	c := defaultProjectConfigInput()
	c.Remote, c.Branch = in.Remote, in.Branch
	c.TaskAuthority, c.WorkflowProtocol = "native", executionmodel.Version
	c.PublicationPolicy.Publisher = backend.PublisherWorker
	c.PR = executionmodel.PRProviderConfig{Kind: in.Provider, BaseURL: in.BaseURL, Repository: in.PRRepository, TokenEnv: in.TokenEnv}
	switch in.Provider {
	case "gitea":
	case "github":
		if in.TokenEnv != "" {
			return invalid("GitHub 使用服务机器上的 gh 认证")
		}
		c.PR.Kind = "command"
		// The browser chooses the provider, never an executable or command arguments.
		c.PR.Query = backend.Command{Executable: "gh", Args: []string{"pr", "view", "{pr_url}", "--json",
			"url,baseRefName,headRefOid,state,mergedAt,mergeCommit", "--jq",
			`{url,repository:"{repository}",base_branch:.baseRefName,head_revision:.headRefOid,state:(if .state=="MERGED" then "merged" elif .state=="CLOSED" then "closed_unmerged" else "open" end),merged:(.state=="MERGED"),merged_at:(if .state=="MERGED" then .mergedAt else null end),merge_commit:(.mergeCommit.oid // null)}`}}
	default:
		return invalid("请选择 Gitea 或 GitHub 发布目标")
	}
	if err := publication.ValidateNonSecret(publication.Config(c.PR), c.Git); err != nil {
		return invalid(err.Error())
	}
	process := NewProcessHost(20 * time.Second)
	git := workspace.Git{Command: backend.LaunchCommand{Executable: c.Git.Executable}, Timeout: 20 * time.Second, Start: process.StartManaged}
	repo, err := workspace.ResolveRepository(ctx, git, in.Repository)
	if err != nil {
		return invalid("无法读取服务机器上的 Git 工作区，请检查路径和访问权限")
	}
	c.Repository = repo.Path
	ref = api.ProjectRef{
		ApplicationHome: d.home,
		ConfigPath:      filepath.Join(d.home, "etc", in.Name+".json"),
		Identity:        canonicalConfigPath(repo.CommonDir),
	}
	ref.RunDir = filepath.Join(d.home, "state", RepositoryStateKey(ref.Identity))
	if !pathguard.Within(d.home, ref.RunDir) {
		return invalid("项目状态路径必须位于当前安装实例内")
	}
	if pathguard.Within(repo.Path, ref.ConfigPath) || pathguard.Within(repo.Path, ref.RunDir) ||
		pathguard.Within(repo.CommonDir, ref.ConfigPath) || pathguard.Within(repo.CommonDir, ref.RunDir) {
		return invalid("项目配置和状态必须位于 Git 工作区及 common-dir 之外")
	}
	branchRef := "refs/heads/" + in.Branch
	if _, err = git.Output(ctx, repo.Path, "check-ref-format", branchRef); err != nil {
		return invalid("目标分支名称无效")
	}
	if _, err = git.Output(ctx, repo.Path, "check-ref-format", "refs/remotes/"+in.Remote+"/branch"); err != nil {
		return invalid("Git remote 名称无效")
	}
	var remoteURLs []string
	for _, options := range [][]string{nil, {"--push"}} {
		args := append(append([]string{"remote", "get-url", "--all"}, options...), "--", in.Remote)
		raw, err := git.Output(ctx, repo.Path, args...)
		url := strings.TrimSpace(string(raw))
		if err != nil || publication.ValidateURL(url) != nil || c.PR.ValidateRemote(url) != nil {
			return invalid("Git remote 的读取和推送地址必须对应所填 PR 目标，凭证由服务机器提供")
		}
		remoteURLs = append(remoteURLs, url)
	}
	remote := commandRunner{Dir: repo.Path, Timeout: 20 * time.Second, ProcessHost: process,
		Env: map[string]string{"GIT_TERMINAL_PROMPT": "0", "GIT_ALLOW_PROTOCOL": "http:https:ssh"}}
	head, err := remote.Output(ctx, c.Git, "ls-remote", "--exit-code", "--refs", "--", in.Remote, branchRef)
	fields := strings.Fields(string(head))
	if err != nil || len(fields) != 2 || fields[1] != branchRef || (len(fields[0]) != 40 && len(fields[0]) != 64) {
		return invalid("无法核验远端目标分支，请检查分支名称、网络和 Git 认证")
	}
	commit := fields[0]
	if _, err = git.Output(ctx, repo.Path, "--no-replace-objects", "cat-file", "-e", commit+"^{commit}"); err != nil {
		return invalid("本地缺少目标分支提交 " + commit + "，请先在服务机器上 fetch，再重新核验")
	}
	reader := taskDocuments{repository: repo.Path, git: c.Git, timeout: 15 * time.Second, processHost: process}
	if err = checkProjectInitialization(materialReads(ctx), reader, commit); err != nil {
		return invalid(err.Error())
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return result, ref, nil, err
	}
	data = append(data, '\n')
	result = contract.ProjectRegistrationResult{
		Repository: repo.Path, Commit: commit, ConfigID: filepath.Base(ref.ConfigPath),
		ProjectID:    httpcontract.ProjectID(ref.Identity),
		Confirmation: loadedConfigDigest(data, []byte(strings.Join(append(remoteURLs, ref.ConfigPath, ref.Identity, commit), "\n"))),
	}
	return result, ref, data, nil
}

func checkProjectInitialization(ctx context.Context, reader taskDocuments, commit string) error {
	docs := []string{"README.md", "AGENTS.md", "SteerlineFlow/paths.md",
		"SteerlineProjectGuideDoc/README.md", "SteerlineProjectGuideDoc/project-initialization.md",
		"SteerlineProjectGuideDoc/project-standards.md", "SteerlineProjectGuideDoc/environment-adaptation.md",
		"SteerlineProjectGuideDoc/spec-plan-guidelines.md", "SteerlineProjectGuideDoc/testing-architecture.md"}
	environment := envmodel.DocumentPath()
	required := append(append([]string{}, docs...), testmodel.ProjectEntryPath, testmodel.ProtocolPath, testmodel.WriterPath, environment)
	files, err := reader.readCommittedFiles(ctx, commit, required)
	if err != nil {
		return fmt.Errorf("无法读取目标提交中的初始化材料：%w", err)
	}
	for _, name := range docs {
		if len(bytes.TrimSpace(files[name])) == 0 || !utf8.Valid(files[name]) {
			return fmt.Errorf("%s 必须包含 UTF-8 项目文档", name)
		}
	}
	rules, err := prompts.TestingRules()
	if err != nil {
		return err
	}
	if !prompts.SameRules(files[testmodel.ProtocolPath], rules) {
		return fmt.Errorf("%s 与安装版本的固定测试协议不同源", testmodel.ProtocolPath)
	}
	if err := verifyTestWriter(func(name string) ([]byte, error) { return files[name], nil }); err != nil {
		return fmt.Errorf("%s：%w", testmodel.WriterPath, err)
	}
	entry, err := testmodel.ParseProjectEntry(files[testmodel.ProjectEntryPath])
	if err != nil {
		return fmt.Errorf("%s：%w", testmodel.ProjectEntryPath, err)
	}
	manifest, err := envmodel.ParseManifest(files[environment])
	if err != nil {
		return fmt.Errorf("%s：%w", environment, err)
	}
	inputs := append(entry.Paths(), manifest.Script)
	inputs = append(inputs, manifest.Dependencies...)
	files, err = reader.readCommittedFiles(ctx, commit, inputs)
	if err != nil {
		return fmt.Errorf("无法读取目标提交中的测试或环境引用：%w", err)
	}
	if len(bytes.TrimSpace(files[manifest.Script])) == 0 {
		return fmt.Errorf("%s 缺少检测入口内容", manifest.Script)
	}
	if _, err := testmodel.ParseCatalogue(files[entry.TestingCatalogue]); err != nil {
		return fmt.Errorf("%s：%w", entry.TestingCatalogue, err)
	}
	return nil
}

func (d *ApplicationDirectory) checkRegistrationConflicts(ctx context.Context, ref api.ProjectRef, data []byte, confirm bool) (bool, error) {
	catalog := loadApplicationCatalog(d.home)
	if catalog.err != nil {
		return false, catalog.err
	}
	saved := false
	for _, entry := range catalog.entries {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if strings.EqualFold(entry.view.ConfigID, filepath.Base(ref.ConfigPath)) {
			if confirm && entry.view.ConfigID == filepath.Base(ref.ConfigPath) && bytes.Equal(entry.data, data) {
				saved = true
				continue
			}
			return false, registrationError(409, "project_name_conflict", "当前安装实例已使用该项目名称")
		}
		other := entry.ref
		if err := projectconnection.ResolveSnapshot(ctx, &other, entry.data); err == nil &&
			(other.Identity == ref.Identity || withinResolved(other.RunDir, ref.RunDir) || withinResolved(ref.RunDir, other.RunDir)) {
			return false, registrationError(409, "project_already_registered", "同一 Git 仓库或项目状态路径已配置为 "+entry.view.ConfigID)
		}
	}
	if saved {
		return true, nil
	}
	if _, err := os.Lstat(ref.RunDir); !errors.Is(err, os.ErrNotExist) {
		return false, registrationError(409, "project_state_exists", "发现已有项目状态或状态路径不可访问，请先核对其归属")
	}
	return false, ctx.Err()
}

func (d *ApplicationDirectory) saveProjectRegistration(path string, data []byte) error {
	dir := filepath.Dir(path)
	resolved, err := pathguard.ResolveProspective(dir)
	if err != nil {
		return err
	}
	home, err := pathguard.ResolveProspective(d.home)
	if err != nil {
		return err
	}
	if canonicalPath(resolved) != canonicalPath(filepath.Join(home, "etc")) {
		return registrationError(409, "configuration_path_changed", "安装实例的配置目录指向其他位置")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := httpfs.CreatePrivateTemp(dir, ".register-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if err = errors.Join(err, f.Close()); err != nil {
		return err
	}
	// Publish the complete file once, without replacing an existing project.
	if err := os.Link(f.Name(), path); errors.Is(err, os.ErrExist) {
		return registrationError(409, "project_name_conflict", "当前安装实例已使用该项目名称")
	} else {
		return err
	}
}
