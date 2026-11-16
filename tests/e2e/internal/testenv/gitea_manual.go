package testenv

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/google/uuid"
)

const ManualVerificationGiteaLiveID = "manual-verification.gitea-local-codex"

const (
	giteaFixtureRepo   = "e2e/verification"
	giteaFixtureToken  = "STEERLINE_E2E_GITEA_TOKEN"
	giteaFixturePrefix = "Review:"
)

func (p *Project) enableManualGitea() {
	s := p.Scenario
	s.Must(p.GitServer.Stop(s.Context))
	dir := filepath.Join(s.Root, "gitea")
	s.Must(os.MkdirAll(dir, 0700))
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	s.Must(err)
	address := listener.Addr().String()
	s.Must(listener.Close())
	_, port, err := net.SplitHostPort(address)
	s.Must(err)
	base := "http://" + address
	config := filepath.Join(dir, "app.ini")
	ini := fmt.Sprintf(`[server]
HTTP_ADDR = 127.0.0.1
HTTP_PORT = %s
ROOT_URL = %s/
DISABLE_SSH = true
[database]
DB_TYPE = sqlite3
PATH = data/gitea.db
[repository]
ROOT = repositories
[repository.pull-request]
WORK_IN_PROGRESS_PREFIXES = %s
[security]
INSTALL_LOCK = true
SECRET_KEY = %s
INTERNAL_TOKEN = %s
[service]
DISABLE_REGISTRATION = true
[indexer]
ISSUE_INDEXER_TYPE = bleve
[actions]
ENABLED = false
[log]
MODE = console
LEVEL = Warn
`, port, base, giteaFixturePrefix, uuid.NewString(), uuid.NewString()+uuid.NewString())
	s.Must(os.WriteFile(config, []byte(ini), 0600))
	command := func(args ...string) *exec.Cmd {
		cmd := exec.Command(s.Run.Gitea, append([]string{"--work-path", dir, "--config", config}, args...)...)
		cmd.Dir = dir
		return cmd
	}
	_, err = s.RunCommandFor(command("migrate"), "gitea-migrate", time.Minute)
	s.Must(err)
	_, err = s.RunCommandFor(command("admin", "user", "create", "--username", "e2e", "--email", "e2e@example.invalid",
		"--password", uuid.NewString(), "--admin", "--must-change-password=false"), "gitea-admin", time.Minute)
	s.Must(err)
	// The short-lived credential belongs only to this fixture. Do not write it to evidence.
	tokenCtx, cancel := context.WithTimeout(s.Context, 30*time.Second)
	defer cancel()
	tokenCommand := command("admin", "user", "generate-access-token", "--username", "e2e", "--scopes", "write:repository,write:user", "--raw")
	tokenProcess := exec.CommandContext(tokenCtx, tokenCommand.Path, tokenCommand.Args[1:]...)
	tokenProcess.Dir = tokenCommand.Dir
	raw, err := tokenProcess.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		s.t.Fatalf("cannot issue isolated Gitea credential: %v: %s", err, exit.Stderr)
	}
	s.Must(err)
	token := strings.TrimSpace(string(raw))
	if len(token) != 40 {
		s.t.Fatal("Gitea fixture did not return a token")
	}
	s.t.Setenv(giteaFixtureToken, token)
	server := s.start(command("web"), "gitea")
	p.Gitea = model.PRProviderConfig{Kind: "gitea", BaseURL: base, Repository: giteaFixtureRepo, TokenEnv: giteaFixtureToken, DraftTitlePrefix: giteaFixturePrefix}
	var version struct{ Version string }
	s.Wait("isolated Gitea ready", func() (bool, error) {
		select {
		case <-server.Done:
			s.t.Fatal("isolated Gitea exited")
		default:
		}
		err := p.giteaRequest(s.Context, "GET", "version", nil, &version, http.StatusOK)
		return version.Version != "" && err == nil, err
	})
	s.Must(p.giteaRequest(s.Context, "POST", "user/repos",
		map[string]any{"name": "verification", "default_branch": p.BaseBranch}, nil, http.StatusCreated))
	p.RemoteURL = base + "/" + giteaFixtureRepo + ".git"
	// Runner preserves repository-scoped Git configuration when it starts the native shell.
	s.t.Setenv("GIT_CONFIG_COUNT", "1")
	s.t.Setenv("GIT_CONFIG_KEY_0", "http."+p.RemoteURL+".extraHeader")
	s.t.Setenv("GIT_CONFIG_VALUE_0", "Authorization: Basic "+base64.StdEncoding.EncodeToString([]byte("e2e:"+token)))
	p.git(p.Repo, "remote", "set-url", "origin", p.RemoteURL)
	var project map[string]any
	raw, err = os.ReadFile(p.Config)
	s.Must(err)
	s.Must(json.Unmarshal(raw, &project))
	project["git"] = map[string]string{"executable": "git"}
	project["pr"] = p.Gitea
	project["pr_poll"] = "1s"
	s.Must(controlled.Write(p.Config, project))
	s.Must(controlled.Write(p.Provider, controlled.State{GitURL: p.RemoteURL}))
	s.Must(controlled.Write(filepath.Join(s.Output, "gitea-version.json"), version))
}

func (p *Project) giteaRequest(ctx context.Context, method, path string, input, output any, status int) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, p.Gitea.BaseURL+"/api/v1/"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "token "+os.Getenv(giteaFixtureToken))
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != status {
		return fmt.Errorf("isolated Gitea %s %s returned HTTP %d, expected %d", method, path, resp.StatusCode, status)
	}
	if output != nil {
		return json.NewDecoder(resp.Body).Decode(output)
	}
	return nil
}

func (p *Project) giteaPR(ctx context.Context, snap Snapshot) (model.PRInfo, error) {
	if len(snap.Runner.Candidates) != 1 || snap.Runner.Candidates[0].PRURL == "" {
		return model.PRInfo{}, nil
	}
	info, err := (publication.QueryClient{Timeout: 10 * time.Second}).Query(ctx, p.Gitea, snap.Runner.Candidates[0].PRURL)
	if err == nil && (info.BaseBranch != p.BaseBranch || info.HeadRepository != p.Gitea.Repository || info.HeadBranch != snap.Runner.Candidates[0].Branch) {
		err = errors.New("Gitea PR differs from the owned candidate")
	}
	return info, err
}

func (p *Project) pushGitea(refspec string) {
	p.git(p.Repo, "push", "origin", refspec)
}

func (p *Project) manualGiteaPublishScript() string {
	return fmt.Sprintf(`param([Parameter(Mandatory=$true)][string]$DraftTitlePrefix)
$ErrorActionPreference = 'Stop'
$branch = git symbolic-ref --short HEAD
if ($LASTEXITCODE -ne 0 -or $branch -ceq '%s') { throw 'expected the candidate branch' }
git push origin ('HEAD:refs/heads/' + $branch)
if ($LASTEXITCODE -ne 0) { throw 'push failed' }
$url = '%s/api/v1/repos/%s/pulls'
$auth = git config --get 'http.%s.extraHeader'
if ($LASTEXITCODE -ne 0 -or !$auth.StartsWith('Authorization: Basic ')) { throw 'fixture authentication unavailable' }
$headers = @{Authorization = $auth.Substring('Authorization: '.Length)}
$existing = @(Invoke-RestMethod -Uri ($url + '?state=open&limit=10') -Headers $headers) | Where-Object { $_.base.ref -ceq '%s' -and $_.head.label -ceq $branch }
if (@($existing).Count -gt 1) { throw 'ambiguous candidate PR' }
if ($existing) { Write-Output $existing.html_url; exit }
$body = @{base='%s'; head=$branch; title=($DraftTitlePrefix + ' Isolated manual verification')} | ConvertTo-Json -Compress
$receipt = Invoke-RestMethod -Method Post -Uri $url -Headers $headers -ContentType 'application/json' -Body $body
Write-Output $receipt.html_url
`, p.BaseBranch, p.Gitea.BaseURL, p.Gitea.Repository, p.RemoteURL, p.BaseBranch, p.BaseBranch)
}

func (p *Project) closeGiteaPR(pr model.PRInfo) {
	number, err := p.Gitea.PRNumber(pr.URL)
	p.Scenario.Must(err)
	p.Scenario.Must(p.giteaRequest(p.Scenario.Context, "PATCH", "repos/"+p.Gitea.Repository+"/pulls/"+strconv.Itoa(number),
		map[string]string{"state": "closed"}, nil, http.StatusCreated))
}
