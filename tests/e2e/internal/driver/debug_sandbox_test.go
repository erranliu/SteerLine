package driver

import (
	"context"
	"encoding/json"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/tests/e2e/internal/native"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDebugPublicationReusesPR(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows publication scripts")
	}
	for _, template := range []string{"projects/SteerlineTest", "orca"} {
		for _, state := range []string{"OPEN", "CLOSED", "MERGED"} {
			t.Run(template+"/"+state, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				root := t.TempDir()
				repo, remote := filepath.Join(root, "repo"), filepath.Join(root, "remote.git")
				git := func(args ...string) string {
					t.Helper()
					out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput()
					if err != nil {
						t.Fatalf("Git fixture: %v: %s", err, out)
					}
					return strings.TrimSpace(string(out))
				}
				git("init", "--bare", remote)
				git("init", "-b", "codex/pr-flow", repo)
				git("-C", repo, "config", "user.name", "Fixture")
				git("-C", repo, "config", "user.email", "fixture@example.invalid")
				git("-C", repo, "remote", "add", "origin", remote)
				file := filepath.Join(repo, "result.txt")
				for _, content := range []string{"original", "updated"} {
					if err := os.WriteFile(file, []byte(content), 0600); err != nil {
						t.Fatal(err)
					}
					git("-C", repo, "add", "result.txt")
					git("-C", repo, "-c", "commit.gpgsign=false", "commit", "-m", content)
					if content == "original" {
						git("-C", repo, "push", "origin", "HEAD")
					}
				}
				oldHead := git("--git-dir", remote, "rev-parse", "refs/heads/codex/pr-flow")
				newHead := git("-C", repo, "rev-parse", "HEAD")
				raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "debug-template", filepath.FromSlash(template), "publish.ps1"))
				if err != nil {
					t.Fatal(err)
				}
				quote := func(s string) string { return strings.ReplaceAll(s, "'", "''") }
				base := "codex/debug/pr-flow/base"
				manifest, _ := json.Marshal(map[string]string{"base_branch": base, "release": "fixture"})
				if err := os.WriteFile(filepath.Join(root, "manifest.json"), manifest, 0600); err != nil {
					t.Fatal(err)
				}
				script := strings.ReplaceAll(strings.ReplaceAll(string(raw), "__ROOT__", quote(filepath.ToSlash(root))), "__BASE__", base)
				publish := filepath.Join(repo, "publish.ps1")
				if err := os.WriteFile(publish, []byte(script), 0600); err != nil {
					t.Fatal(err)
				}
				// Stub only GitHub facts. Git push really updates this isolated bare repo.
				// The provider's stale head must not block reuse or cause another PR.
				url := "https://github.com/erranliu/SteerlineTest/pull/84"
				wrapper := `function gh.exe {}
function gh {
    $global:LASTEXITCODE = 0
    switch ($args[1]) {
        'list' {
            if ($args -notcontains 'all') { '[]'; return }
            '[{"url":"` + url + `"}]'
        }
        'view' { '{"url":"` + url + `","state":"` + state + `","headRefOid":"` + oldHead + `","headRefName":"codex/pr-flow","baseRefName":"` + base + `"}' }
        default { throw 'Unexpected new PR or GitHub write' }
    }
}
Set-Location '` + quote(repo) + `'
& '` + quote(publish) + `'
`
				path := filepath.Join(root, "verify.ps1")
				if err := os.WriteFile(path, []byte(wrapper), 0600); err != nil {
					t.Fatal(err)
				}
				out, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", path).CombinedOutput()
				if state != "OPEN" {
					if err == nil || !strings.Contains(string(out), "PR identity mismatch") || strings.Contains(string(out), "Unexpected new PR") {
						t.Fatalf("closed/merged PR was recreated or treated as open: %v: %s", err, out)
					}
					return
				}
				if err != nil || !strings.Contains(string(out), url) {
					t.Fatalf("publication failed: %v: %s", err, out)
				}
				if got := git("--git-dir", remote, "rev-parse", "refs/heads/codex/pr-flow"); got != newHead || got == oldHead {
					t.Fatal("existing PR was returned without pushing the new commit")
				}
			})
		}
	}
}

func TestDebugReleaseCodexSandbox(t *testing.T) {
	root := os.Getenv("STEERLINE_DEBUG_SANDBOX_ROOT")
	if root == "" {
		t.Skip("run scripts/test-codex-sandbox.ps1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	if _, err := RunDebugSandbox(ctx, root, ""); err != nil {
		t.Fatal(err)
	}
}

func TestDebugSandboxStageCommands(t *testing.T) {
	for _, tc := range []struct {
		name, step, command string
		wantError           bool
	}{
		{"ordinary_probe", executionmodel.StepImplement, "powershell.exe -NoProfile -File ./sandbox-check.ps1", false},
		{"read_publication_script", executionmodel.StepImplement, "Get-Content ./publish.ps1", false},
		{"premature_script", executionmodel.StepImplement, "pwsh -Command 'powershell.exe -NoProfile -File ./publish.ps1'", true},
		{"premature_windows_script", executionmodel.StepImplement, `powershell.exe -File .\publish.ps1`, true},
		{"premature_git_push", executionmodel.StepImplement, "git -c credential.helper= push origin HEAD:refs/heads/codex/task", true},
		{"premature_pr", executionmodel.StepImplement, "gh pr create --repo erranliu/SteerlineTest", true},
		{"report_without_tools", executionmodel.StepReport, "", false},
		{"report_with_tools", executionmodel.StepReport, "git status --short", true},
		{"publication_without_tools", executionmodel.StepPublish, "", true},
		{"publication", executionmodel.StepPublish, "powershell.exe -NoProfile -File ./publish.ps1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var commands []native.RolloutCommand
			if tc.command != "" {
				commands = append(commands, native.RolloutCommand{Command: []string{tc.command}})
			}
			if err := debugSandboxStageCommands(commands, tc.step); (err != nil) != tc.wantError {
				t.Fatalf("stage evidence error=%v, wantError=%v", err, tc.wantError)
			}
		})
	}
}
