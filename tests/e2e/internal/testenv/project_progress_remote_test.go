//go:build e2e

package testenv

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/erranliu/SteerLine/internal/runnercontract"
	process "github.com/erranliu/SteerLine/internal/testkit/process"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/google/uuid"
)

type progressRemote struct {
	Profile                                            map[string]any
	Executable, Environment, Runtime, Repository, Root string
	sequence                                           int
}

func loadProgressRemote(s *Scenario, p *Project) *progressRemote {
	path := os.Getenv("STEERLINE_PROGRESS_REMOTE_PROFILE")
	if path == "" {
		return nil
	}
	if !filepath.IsAbs(path) {
		s.t.Fatal("remote profile must be absolute")
	}
	r := &progressRemote{}
	raw, err := os.ReadFile(path)
	s.Must(err)
	s.Must(json.Unmarshal(raw, &r.Profile))
	execution, ok := r.Profile["execution"].(map[string]any)
	if !ok || execution["adapter"] != "orca-terminal" {
		s.t.Fatal("requires explicit remote Orca execution")
	}
	target, ok := execution["target"].(map[string]any)
	if !ok || target["kind"] != "orca_environment" {
		s.t.Fatal("local execution cannot qualify this test")
	}
	r.Environment, _ = target["environment"].(string)
	settings, ok := execution["settings"].(map[string]any)
	if !ok {
		s.t.Fatal("remote settings missing")
	}
	transport, ok := settings["transport"].(map[string]any)
	if !ok {
		s.t.Fatal("remote transport missing")
	}
	r.Executable, _ = transport["executable"].(string)
	if r.Environment == "" || !filepath.IsAbs(r.Executable) {
		s.t.Fatal("remote transport/environment must be explicit")
	}
	var list struct {
		Repos []struct {
			ID, Path string
			Identity struct {
				URL string `json:"remoteUrl"`
			} `json:"gitRemoteIdentity"`
		} `json:"repos"`
	}
	body, err := r.Call(s.Context, s, p, "repo", "list")
	s.Must(err)
	s.Must(json.Unmarshal(body, &list))
	for _, repo := range list.Repos {
		if strings.EqualFold(strings.TrimSuffix(repo.Identity.URL, ".git"), strings.TrimSuffix(githubURL, ".git")) {
			if r.Repository != "" {
				s.t.Fatal("remote repository identity is ambiguous")
			}
			r.Repository, r.Root = repo.ID, repo.Path
		}
	}
	if r.Repository == "" {
		s.t.Fatal("SteerlineTest is not registered on the selected remote host")
	}
	s.Must(controlled.Write(filepath.Join(s.Output, "remote-target.json"), map[string]any{"environment": r.Environment, "runtime": r.Runtime, "repository": r.Repository, "root": r.Root, "profile": r.Profile}))
	return r
}

func (r *progressRemote) Call(ctx context.Context, s *Scenario, p *Project, args ...string) (json.RawMessage, error) {
	r.sequence++
	cmd := exec.Command(r.Executable, append(args, "--environment", r.Environment, "--json")...)
	cmd.Env = p.Driver.Environment(os.Environ())
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := process.Run(ctx, cmd)
	name := filepath.Join(s.Output, fmt.Sprintf("orca-%03d.json", r.sequence))
	if saveErr := os.WriteFile(name, stdout.Bytes(), 0600); saveErr != nil {
		return nil, saveErr
	}
	if err != nil {
		return nil, fmt.Errorf("Orca %s failed: %w: %s", strings.Join(args[:min(2, len(args))], " "), err, stderr.String())
	}
	var envelope struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
		Meta   struct {
			Runtime string `json:"runtimeId"`
		} `json:"_meta"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		return nil, err
	}
	if !envelope.OK {
		return nil, fmt.Errorf("Orca rejected operation: %s", envelope.Error)
	}
	if r.Runtime == "" {
		r.Runtime = envelope.Meta.Runtime
	}
	if r.Runtime == "" || envelope.Meta.Runtime != r.Runtime {
		return nil, errors.New("remote runtime identity changed")
	}
	return envelope.Result, nil
}

func psProgressLiteral(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

// An owned diagnostic shell reads only this test's references and reports. It
// never writes the progress file or sends input to the original worker terminal.
func (r *progressRemote) Shell(ctx context.Context, s *Scenario, p *Project, workspace, script string) (json.RawMessage, error) {
	nonce := strings.ReplaceAll(uuid.NewString(), "-", "")
	body := "$ErrorActionPreference='Stop'; try { $value=& {\n" + script + "\n}; $result=@{passed=$true;value=$value} } catch { $result=@{passed=$false;error=$_.Exception.Message} }; " +
		"$encoded=[Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes(($result|ConvertTo-Json -Depth 12 -Compress))); $index=0; for($offset=0;$offset -lt $encoded.Length;$offset+=512){ Write-Output ('SLP_" + nonce + ":'+$index+':'+$encoded.Substring($offset,[Math]::Min(512,$encoded.Length-$offset))); $index++ }; Write-Output ('SLE_" + nonce + ":'+$index)"
	words := utf16.Encode([]rune(body))
	encoded := make([]byte, len(words)*2)
	for i, v := range words {
		binary.LittleEndian.PutUint16(encoded[i*2:], v)
	}
	selector := "id:" + r.Repository + "::" + strings.ReplaceAll(workspace, "\\", "/")
	raw, err := r.Call(ctx, s, p, "terminal", "create", "--worktree", selector, "--title", "SteerLine progress evidence "+nonce[:8], "--command", "powershell.exe -NoProfile -NonInteractive -EncodedCommand "+base64.StdEncoding.EncodeToString(encoded))
	if err != nil {
		return nil, err
	}
	var created struct {
		Terminal struct {
			Handle string `json:"handle"`
		} `json:"terminal"`
	}
	if err = json.Unmarshal(raw, &created); err != nil {
		return nil, err
	}
	if created.Terminal.Handle == "" {
		return nil, errors.New("original diagnostic terminal handle missing")
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		closed, err := r.Call(closeCtx, s, p, "terminal", "close", "--terminal", created.Terminal.Handle)
		var receipt struct {
			Close struct {
				Killed bool `json:"ptyKilled"`
			} `json:"close"`
		}
		if err == nil {
			err = json.Unmarshal(closed, &receipt)
			if err == nil && !receipt.Close.Killed {
				err = errors.New("diagnostic terminal stop unverified")
			}
		}
		if err != nil {
			s.t.Error(err)
		}
	}()
	chunkPattern := regexp.MustCompile("SLP_" + nonce + `:(\d+):([A-Za-z0-9+/=]+)`)
	donePattern := regexp.MustCompile("SLE_" + nonce + `:(\d+)`)
	chunks := map[int]string{}
	total := -1
	cursor := ""
	for {
		args := []string{"terminal", "read", "--terminal", created.Terminal.Handle, "--limit", "2000"}
		if cursor != "" {
			args = append(args, "--cursor", cursor)
		}
		read, err := r.Call(ctx, s, p, args...)
		if err != nil {
			return nil, err
		}
		var view struct {
			Terminal struct {
				Tail []string `json:"tail"`
				Next string   `json:"nextCursor"`
			} `json:"terminal"`
		}
		if err = json.Unmarshal(read, &view); err != nil {
			return nil, err
		}
		cursor = view.Terminal.Next
		for _, line := range view.Terminal.Tail {
			if m := chunkPattern.FindStringSubmatch(line); m != nil {
				i, _ := strconv.Atoi(m[1])
				if i < 0 || i > 2048 {
					return nil, errors.New("remote evidence exceeds bound")
				}
				chunks[i] = m[2]
			}
			if m := donePattern.FindStringSubmatch(line); m != nil {
				total, _ = strconv.Atoi(m[1])
			}
		}
		if total >= 0 && len(chunks) == total {
			var joined strings.Builder
			for i := 0; i < total; i++ {
				v, ok := chunks[i]
				if !ok {
					return nil, errors.New("remote evidence chunk missing")
				}
				joined.WriteString(v)
			}
			decoded, err := base64.StdEncoding.DecodeString(joined.String())
			if err != nil {
				return nil, err
			}
			var result struct {
				Passed bool            `json:"passed"`
				Value  json.RawMessage `json:"value"`
				Error  string          `json:"error"`
			}
			if err = json.Unmarshal(decoded, &result); err != nil {
				return nil, err
			}
			if !result.Passed {
				return nil, errors.New("remote evidence operation: " + result.Error)
			}
			return result.Value, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (r *progressRemote) Fetch(s *Scenario, p *Project, head string) {
	script := "$root=" + psProgressLiteral(r.Root) + "; $ref=" + psProgressLiteral("refs/remotes/origin/"+p.BaseBranch) + "; $before=(& git -C $root rev-parse HEAD|Out-String).Trim(); if($LASTEXITCODE){throw 'original root HEAD unavailable'}; $statusBefore=(& git -C $root status --porcelain|Out-String); if($LASTEXITCODE){throw 'original root status unavailable'}; $ErrorActionPreference='Continue'; $fetch=@(& git -C $root fetch --no-tags origin " + psProgressLiteral("refs/heads/"+p.BaseBranch+":refs/remotes/origin/"+p.BaseBranch) + " 2>&1); $fetchExit=$LASTEXITCODE; $ErrorActionPreference='Stop'; if($fetchExit){throw 'owned base fetch failed'}; $actual=(& git -C $root rev-parse $ref|Out-String).Trim(); if($LASTEXITCODE -or $actual -cne " + psProgressLiteral(head) + "){throw 'remote owned base differs'}; $after=(& git -C $root rev-parse HEAD|Out-String).Trim(); $statusAfter=(& git -C $root status --porcelain|Out-String); if($before -cne $after -or $statusBefore -cne $statusAfter){throw 'registered root worktree changed'}; @{commit=$actual;original_root_unchanged=$true;head=$after}"
	raw, err := r.Shell(s.Context, s, p, r.Root, script)
	s.Must(err)
	s.Must(os.WriteFile(filepath.Join(s.Output, "remote-fetch.json"), raw, 0600))
}

func (r *progressRemote) Capture(s *Scenario, p *Project, run runnercontract.Run, required bool) string {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	script := `$files=@{}; $paths=@('tools/test-runtime-progress.py','tools/runtime_progress.py','tools/steerline_reports.py'); $reports=Join-Path (Get-Location) '.steerrunner/artifacts/live-progress'; if(Test-Path -LiteralPath $reports){ foreach($file in @(Get-ChildItem -LiteralPath $reports -File -Filter '*.json')) { $paths+=('.steerrunner/artifacts/live-progress/'+$file.Name) } }; foreach($path in $paths){ $bytes=[IO.File]::ReadAllBytes((Join-Path (Get-Location) $path)); if($bytes.Length -gt 65536){throw 'original file exceeds evidence bound'}; $files[$path]=[Convert]::ToBase64String($bytes) }; $head=(& git rev-parse HEAD|Out-String).Trim(); if($LASTEXITCODE){throw 'original HEAD unavailable'}; $status=@(& git status --porcelain); if($LASTEXITCODE){throw 'original status unavailable'}; @{workspace=(Get-Location).Path;head=$head;clean=($status.Count -eq 0);files=$files}`
	raw, err := r.Shell(ctx, s, p, run.Execution.Workspace, script)
	if err != nil {
		if required {
			s.Must(err)
		} else {
			s.t.Log("remote diagnostic capture:", err)
		}
		return ""
	}
	var value struct {
		Workspace string            `json:"workspace"`
		Head      string            `json:"head"`
		Clean     bool              `json:"clean"`
		Files     map[string]string `json:"files"`
	}
	s.Must(json.Unmarshal(raw, &value))
	if required && (!value.Clean || value.Head != p.git(p.Repo, "rev-parse", "HEAD")) {
		s.t.Fatal("remote original workspace must remain clean at the pinned fixture commit")
	}
	if !strings.EqualFold(strings.ReplaceAll(value.Workspace, "\\", "/"), strings.ReplaceAll(run.Execution.Workspace, "\\", "/")) {
		s.t.Fatal("remote capture workspace differs")
	}
	s.Must(os.WriteFile(filepath.Join(s.Output, "remote-source.json"), raw, 0600))
	root := filepath.Join(s.Output, "remote-workspace")
	for path, encoded := range value.Files {
		allowed := path == "tools/test-runtime-progress.py" || path == "tools/runtime_progress.py" || path == "tools/steerline_reports.py" || regexp.MustCompile(`^\.steerrunner/artifacts/live-progress/(summary|managed-[0-9a-f]{32})\.json$`).MatchString(path)
		if !allowed {
			s.t.Fatal("unexpected remote evidence path", path)
		}
		body, err := base64.StdEncoding.DecodeString(encoded)
		s.Must(err)
		if len(body) > 65536 {
			s.t.Fatal("remote file exceeds evidence bound")
		}
		destination := filepath.Join(root, filepath.FromSlash(path))
		s.Must(os.MkdirAll(filepath.Dir(destination), 0700))
		s.Must(os.WriteFile(destination, body, 0600))
	}
	return root
}

func readLocalProgressSummary(remote *progressRemote, reports string) ([]byte, error) {
	if remote != nil {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(filepath.Join(reports, "summary.json"))
}
