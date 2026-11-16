//go:build e2e

package testenv

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
)

// The registered source may have user sessions. Clone the fixture branch into
// a separate registration so checkout preparation never takes over that source.
func (r *orcaTarget) isolateMaintenanceCheckout(s *Scenario, p *Project, head string) {
	parts := strings.Split(p.BaseBranch, "/")
	if len(parts) != 5 || len(parts[2]) != 32 {
		s.t.Fatal("invalid owned base identity")
	}
	name := "sl-maintenance-" + parts[2][:12]
	script := "$name=" + psProgressLiteral(name) + "; $path=Join-Path ([IO.Path]::GetFullPath($env:TEMP)) $name; " +
		"if(Test-Path -LiteralPath $path){throw 'owned checkout already exists'}; " +
		"& git -c credential.helper= -c 'credential.helper=!gh auth git-credential' clone --quiet --no-tags --single-branch --branch " +
		psProgressLiteral(p.BaseBranch) + " " + psProgressLiteral(githubURL) + " $path; " +
		"if($LASTEXITCODE){throw 'owned clone failed'}; " +
		"& git -C $path config --local user.name 'SteerLine E2E'; if($LASTEXITCODE){throw 'fixture git user setup failed'}; " +
		"& git -C $path config --local user.email 'steerline-e2e@example.invalid'; if($LASTEXITCODE){throw 'fixture git email setup failed'}; " +
		"& git -C $path config --local commit.gpgsign false; if($LASTEXITCODE){throw 'fixture commit setup failed'}; " +
		"$actual=(& git -C $path rev-parse HEAD|Out-String).Trim(); " +
		"if($LASTEXITCODE -or $actual -cne " + psProgressLiteral(head) + "){throw 'owned clone head differs'}; @{path=$path;head=$actual}"
	bootstrap := *r
	raw, err := r.Shell(s.Context, s, p, bootstrap.Root, script)
	s.Must(err)
	var clone struct{ Path, Head string }
	s.Must(json.Unmarshal(raw, &clone))
	if clone.Path == "" || clone.Head != head || !strings.EqualFold(filepath.Base(clone.Path), name) {
		s.t.Fatal("owned remote checkout identity unverified")
	}
	s.Must(controlled.Write(filepath.Join(s.Output, "remote-checkout-intent.json"), clone))
	raw, err = r.Call(s.Context, s, p, "repo", "add", "--path", clone.Path, "--kind", "git")
	s.Must(err)
	var added struct{ Repo struct{ ID, Path string } }
	s.Must(json.Unmarshal(raw, &added))
	if added.Repo.ID == "" || added.Repo.ID == bootstrap.Repository || !sameOrcaPath(added.Repo.Path, clone.Path) {
		s.t.Fatal("isolated remote registration differs")
	}
	r.Repository, r.Root = added.Repo.ID, added.Repo.Path
	s.Must(controlled.Write(filepath.Join(s.Output, "remote-checkout.json"), map[string]string{"path": r.Root, "setup": r.Repository, "head": head, "environment": r.Environment}))
	p.RemoveRemoteFixture = func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		// Delete only the independently registered fixture and its managed worktrees.
		if _, err := r.Call(ctx, s, p, "project", "setup-delete", "--setup", r.Repository, "--force"); err != nil {
			return err
		}
		body, err := r.Call(ctx, s, p, "repo", "list")
		if err != nil {
			return err
		}
		var inventory struct {
			Repos     []struct{ ID, Path string }
			Truncated bool
		}
		if json.Unmarshal(body, &inventory) != nil || inventory.Repos == nil || inventory.Truncated {
			return errors.New("remote fixture registration cleanup unverified")
		}
		for _, repo := range inventory.Repos {
			if repo.ID == r.Repository || sameOrcaPath(repo.Path, clone.Path) {
				return errors.New("remote fixture registration still present")
			}
		}
		bootstrap.sequence = r.sequence
		script := "$path=" + psProgressLiteral(clone.Path) + "; $expected=Join-Path ([IO.Path]::GetFullPath($env:TEMP)) " + psProgressLiteral(name) + "; " +
			"if($path -ine $expected){throw 'owned checkout outside exact temp path'}; " +
			"if(Test-Path -LiteralPath $path){$item=Get-Item -LiteralPath $path -Force; " +
			"if ($item.FullName -ine $expected -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)){throw 'owned checkout redirected'}; " +
			"Remove-Item -LiteralPath $item.FullName -Recurse -Force}; " +
			"if(Test-Path -LiteralPath $path){throw 'owned checkout remains'}; @{removed=$true}"
		body, err = bootstrap.Shell(ctx, s, p, bootstrap.Root, script)
		r.sequence = bootstrap.sequence
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(s.Output, "remote-checkout-cleanup.json"), body, 0600)
	}

	repo := filepath.Join(p.Root, name)
	s.Must(os.Rename(p.Repo, repo))
	p.Repo = repo
	var config map[string]any
	body, err := os.ReadFile(p.Config)
	s.Must(err)
	s.Must(json.Unmarshal(body, &config))
	config["repository"] = repo
	s.Must(controlled.Write(p.Config, config))
	body, err = p.Driver.Run(s.Context, "home", "-config", p.Config)
	s.Must(err)
	p.Database = ""
	for _, line := range strings.Split(string(body), "\n") {
		if path, ok := strings.CutPrefix(line, "run_dir "); ok {
			p.Database = filepath.Join(strings.TrimSpace(path), "steerline.db")
		}
	}
	if !filepath.IsAbs(p.Database) {
		s.t.Fatal("isolated fixture database path missing")
	}
}
