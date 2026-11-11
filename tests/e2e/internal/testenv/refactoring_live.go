package testenv

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/billing"
	"github.com/erranliu/SteerLine/internal/billingconfig"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	ref "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func RefactoringLive(s *Scenario) {
	p := s.NewLiveProject("maintenance-live", "local-codex")
	// Copy only login into this owned home; do not run sessions in the user's home.
	auth, e := os.ReadFile(filepath.Join(s.Run.AuthHome, "auth.json"))
	s.Must(e)
	native := filepath.Join(p.Home, "native-auth")
	s.Must(os.MkdirAll(native, 0700))
	s.Must(os.WriteFile(filepath.Join(native, "auth.json"), auth, 0600))
	p.Driver.NativeHome = native
	files := map[string]string{
		"AGENTS.md": "# Isolated maintenance fixture\nMaintenance may change only alpha.cjs and beta.cjs. Preserve the calculate exports and their behavior. The validation entry point is node check.cjs. Keep existing checks and project materials intact.\n",
		"alpha.cjs": "exports.calculate = function(values) { let result = 0; for (let i = 0; i < values.length; i++) { if (values[i] > 0) { result = result + values[i]; } else { result = result; } } return result; };\n",
		"beta.cjs":  "exports.calculate = function(values) { const output = []; for (let i=0;i<values.length;i++) { if (values[i] !== null) { if (values[i] !== undefined) { output.push(values[i]); } } } return output; };\n",
		"check.cjs": `const assert = require('node:assert/strict');
const a = require('./alpha.cjs');
const b = require('./beta.cjs');
assert.equal(a.calculate([-1, 0, 2, 3]), 5);
assert.equal(a.calculate([]), 0);
assert.equal(a.calculate(['1', -1]), '01');
assert.deepEqual(b.calculate([0, null, false, undefined, '']), [0, false, '']);
assert.deepEqual(b.calculate([]), []);
let reads = 0;
const accessor = [0];
Object.defineProperty(accessor, '0', { get: () => ++reads === 2 ? undefined : 7 });
assert.deepEqual(b.calculate(accessor), []);
assert.equal(reads, 2);
console.log('checks passed');
`,
	}
	for path, body := range files {
		s.Must(os.WriteFile(filepath.Join(p.Repo, path), []byte(body), 0600))
	}
	p.git(p.Repo, "add", ".")
	p.git(p.Repo, "commit", "--quiet", "-m", "Prepare bounded maintenance fixture")
	p.git(p.Repo, "push", "--quiet", "origin", p.BaseBranch)
	price := billing.Policy{Source: "E2E declared reference rates, not provider prices", Currency: "USD", Basis: billing.Basis, Rules: []billing.Rule{{ID: "fixture", Provider: "fixture-reference", Model: "gpt-6-luna", From: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), Rates: billing.Rates{Input: "1", Read: "1", Write: "1", Output: "1"}}}}
	_, e = (billingconfig.Store{Home: p.Home}).Save("*", price)
	s.Must(e)
	p.CreateTasks()
	baseline := p.git(p.Repo, "rev-parse", "HEAD")
	p.validateControlledEnvironment("local-codex", baseline)
	verifyTree := func(report ref.Report, stage string, concurrent bool) {
		p.git(p.Repo, "fetch", "--quiet", "origin", report.Head)
		p.git(p.Repo, "merge-base", "--is-ancestor", report.Base, report.Head)
		if changed := p.git(p.Repo, "diff", "--name-only", report.Base, report.Head); strings.Join(strings.Fields(changed), " ") != "alpha.cjs beta.cjs" {
			s.t.Fatalf("%s must retain both simplifications and modify only their files: %q", stage, changed)
		}
		tree := filepath.Join(s.Output, stage+"-tree")
		s.Must(os.MkdirAll(tree, 0700))
		for _, file := range []string{"alpha.cjs", "beta.cjs", "check.cjs"} {
			body := p.git(p.Repo, "show", report.Head+":"+file)
			compact := strings.Join(strings.Fields(body), "")
			if file == "alpha.cjs" && strings.Contains(compact, "result=result;") || file == "beta.cjs" && strings.Count(compact, "if(") > 1 {
				s.t.Fatalf("%s still contains the original redundancy in %s", stage, file)
			}
			if concurrent && file != "check.cjs" && !strings.Contains(body, "// Concurrent task change: retain this comment.") {
				s.t.Fatalf("%s lost the concurrent change in %s", stage, file)
			}
			s.Must(os.WriteFile(filepath.Join(tree, file), []byte(body+"\n"), 0600))
		}
		cmd := p.Command("node", "check.cjs")
		cmd.Dir = tree
		_, err := s.RunCommand(cmd, stage+"-tree-check")
		s.Must(err)
		s.Must(controlled.Write(filepath.Join(s.Output, stage+"-report.json"), report))
	}
	verifyPR := func(url string, report ref.Report) {
		var pr struct {
			Body  string
			Head  string `json:"headRefOid"`
			Base  string `json:"baseRefName"`
			Draft bool   `json:"isDraft"`
		}
		raw, err := p.gh(s.Context, "pr", "view", url, "--repo", githubRepository, "--json", "body,headRefOid,baseRefName,isDraft")
		s.Must(err)
		s.Must(json.Unmarshal(raw, &pr))
		if pr.Body != report.Body || pr.Head != report.Head || pr.Base != p.BaseBranch || !pr.Draft {
			s.t.Fatal("draft PR and report source mismatch")
		}
	}
	project, e := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
	s.Must(e)
	identity := project.Identity()
	project.Close()
	client, e := applicationclient.Open(s.Context, p.Home)
	s.Must(e)
	defer client.Close()
	prefix := "/projects/" + identity.ProjectID
	post := func(action, target string, revision uint64, payload any) (string, httpcontract.CommandResponse) {
		body, e := json.Marshal(payload)
		s.Must(e)
		id := ref.ID()
		request := httpcontract.CommandEnvelope{RequestID: id, ProjectID: identity.ProjectID, InstanceID: identity.InstanceID, Action: action, TargetID: target, ExpectedVersion: revision, Payload: body}
		var response httpcontract.CommandResponse
		s.Must(client.Post(s.Context, prefix+"/commands", request, &response))
		return id, response
	}
	id, started := post("refactoring_start", identity.RepositoryIdentity, 1, ref.Start{Direction: "simplify", Requirements: "每轮只简化 alpha.cjs 或 beta.cjs 一个文件，完成两个独立切片；保留 calculate 导出及行为。验证 node check.cjs，不能修改检查或项目材料。交付一个 draft PR，报告原样放入原 PR 正文。", Profile: "local-codex", Strategy: "cautious", Budget: ref.Budget{Rounds: 2, Minutes: "25"}})
	if started.Status != "applied" {
		s.t.Fatalf("refactoring_start: %s", started.Diagnostic)
	}
	var view struct {
		ID                string
		Revision          uint64
		Phase, Diagnostic string
		Round             int
		Reports           []ref.Report
		PR                *struct{ URL string }
		Question          *ref.Question
		Operations        []struct {
			Purpose, State, Feedback string
			Round                    int
			RunID                    string `json:"run_id"`
			Result                   *ref.Result
			Cost                     ref.Cost
		}
	}
	defer func() { s.Must(controlled.Write(filepath.Join(s.Output, "maintenance-view.json"), view)) }()
	wait := func(reports int) {
		s.Wait("native maintenance delivery", func() (bool, error) {
			e := client.Get(s.Context, prefix+"/refactoring/"+id, &view)
			if e == nil && (view.Phase == "blocked" || view.Phase == "waiting_input" || view.Phase == "stopped") {
				s.t.Fatalf("native maintenance needs decision: %s %+v", view.Diagnostic, view.Question)
			}
			return e == nil && view.Phase == "delivered" && len(view.Reports) == reports, e
		})
	}
	wait(1)
	if view.Round != 2 || view.PR == nil || view.Reports[0].Base != baseline {
		s.t.Fatal("two rounds and PR required", view)
	}
	original := view.PR.URL
	initial := view.Reports[0]
	verifyPR(original, initial)
	verifyTree(initial, "initial", false)
	// Advance the owned base with an overlapping replacement; the original PR must be updated.
	for _, name := range []string{"alpha.cjs", "beta.cjs"} {
		body := strings.ReplaceAll(strings.ReplaceAll(files[name], "let result = 0", "let result = Number(0)"), "const output = [];", "const output = Array();")
		s.Must(os.WriteFile(filepath.Join(p.Repo, name), []byte("// Concurrent task change: retain this comment.\n"+body), 0600))
	}
	p.git(p.Repo, "add", "alpha.cjs", "beta.cjs")
	p.git(p.Repo, "commit", "--quiet", "-m", "Advance owned base during maintenance")
	p.git(p.Repo, "push", "--quiet", "origin", p.BaseBranch)
	advanced := p.git(p.Repo, "rev-parse", "HEAD")
	merge := exec.CommandContext(s.Context, "git", "merge-tree", "--write-tree", advanced, initial.Head)
	merge.Dir = p.Repo
	merge.Env = p.Driver.Environment(os.Environ())
	proof, mergeErr := merge.CombinedOutput()
	if mergeErr == nil || !strings.Contains(string(proof), "CONFLICT") {
		s.t.Fatal("fixture needs actual Git conflict", mergeErr, string(proof))
	}
	s.Must(os.WriteFile(filepath.Join(s.Output, "conflict-proof.txt"), proof, 0600))
	for attempt := 0; ; attempt++ {
		// GitHub and Git checks may outlast a late usage refresh of the action.
		s.Must(client.Get(s.Context, prefix+"/refactoring/"+id, &view))
		if view.ID != id || view.Phase != "delivered" || view.PR == nil || view.PR.URL != original || len(view.Reports) != 1 ||
			view.Reports[0].Head != initial.Head || view.Reports[0].Base != initial.Base || view.Reports[0].Body != initial.Body {
			s.t.Fatal("conflict action changed after initial delivery")
		}
		_, response := post("refactoring_conflict", id, view.Revision, struct{}{})
		if response.Status == "applied" {
			break
		}
		if attempt != 0 || response.Status != "rejected" || response.Diagnostic != ref.ErrConflict.Error() {
			s.t.Fatalf("refactoring_conflict: %s", response.Diagnostic)
		}
	}
	wait(2)
	if view.PR.URL != original || view.Round != 2 {
		s.t.Fatal("original PR/round changed")
	}
	last := view.Reports[len(view.Reports)-1]
	if last.Base != advanced || !strings.Contains(last.Body, "冲突") {
		s.t.Fatal("report/platform source mismatch")
	}
	verifyPR(original, last)
	verifyTree(last, "conflict", true)
	var prs []struct{ URL string }
	raw, e := p.gh(s.Context, "pr", "list", "--repo", githubRepository, "--base", p.BaseBranch, "--state", "all", "--limit", "2", "--json", "url")
	s.Must(e)
	s.Must(json.Unmarshal(raw, &prs))
	if len(prs) != 1 || prs[0].URL != original {
		s.t.Fatal("maintenance must publish exactly one original PR")
	}
	reader, e := sqlite.OpenSQLiteReader(p.Database)
	s.Must(e)
	defer reader.Close()
	records, e := reader.Refactorings(s.Context, p.ProjectID)
	s.Must(e)
	if len(records) != 1 {
		s.t.Fatal("one action required")
	}
	r := records[0]
	latest, origin, identityKey, workspace, branch := "", "", "", "", ""
	writes := 0
	closingSeen := false
	sessions := map[string]bool{}
	var gitFacts []*runnercontract.WorkspaceGit
	for _, o := range r.Operations {
		if o.Run == nil || o.Run.Workflow.ID != "execute" {
			continue
		}
		closingSeen = closingSeen || o.Purpose == "publish" || o.Purpose == "conflict"
		if !closingSeen && o.Result != nil && o.Result.PR != "" {
			s.t.Fatal("native execute returned a PR before a closing operation", o.Purpose, o.Result.PR)
		}
		writes++
		run := o.Run
		exec := run.Execution
		if exec.Workspace == "" || exec.Branch == "" || run.WorkspaceIdentity == "" || run.WorkspaceOriginRunID == "" || exec.SessionID == "" || sessions[exec.SessionID] {
			s.t.Fatal("native execute did not bind a managed workspace and a new session")
		}
		if run.ReuseWorktreeFromRunID != latest || latest != "" && (run.WorkspaceIdentity != identityKey || run.WorkspaceOriginRunID != origin || exec.Workspace != workspace || exec.Branch != branch) {
			s.t.Fatal("native reuse chain differs")
		}
		latest, origin, identityKey, workspace, branch = run.RunID, run.WorkspaceOriginRunID, run.WorkspaceIdentity, exec.Workspace, exec.Branch
		sessions[exec.SessionID] = true
		if o.WorkspaceGit != nil {
			gitFacts = append(gitFacts, o.WorkspaceGit)
		}
	}
	if writes < 4 {
		s.t.Fatal("native execute chain incomplete")
	}
	if len(gitFacts) < 2 {
		s.t.Fatal("missing verified workspace evidence from native Runner")
	}
	amount, complete := r.CostTotal()
	if amount == nil {
		s.t.Fatal("missing observed reference usage")
	}
	s.Must(controlled.Write(filepath.Join(s.Output, "refactoring-live-proof.json"), map[string]any{"rounds": r.Round, "executes": writes, "same_workspace": true, "workspace_git": gitFacts, "original_pr": original, "report_matches_pr": true, "conflict_audit": true, "verified_both_trees": true, "amount": amount, "cost_complete": complete, "pricing": "declared fixture reference rates", "head": last.Head, "base": last.Base}))
	s.Must(s.FinishBody())
}
