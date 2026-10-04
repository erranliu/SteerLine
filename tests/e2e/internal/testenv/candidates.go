package testenv

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
	"github.com/erranliu/SteerLine/tests/e2e/internal/evidence"
	"github.com/erranliu/SteerLine/tests/e2e/internal/native"
	"github.com/google/uuid"
)

type Candidate struct{ RunID, ExecutionID, ProfileID string }

func (s *Scenario) CandidateManifest() native.CandidateManifest {
	m, err := native.LoadCandidateManifest(s.Run.CandidateConfig)
	if err == nil {
		err = m.Preflight(s.Context, s.Run.Root, s.Run.AuthHome, s.ID)
	}
	if err != nil {
		s.Must(controlled.Write(filepath.Join(s.Output, "blocker.json"), map[string]any{"scenario": s.ID, "diagnostic": err.Error(), "before_model_and_external_resources": true, "manifest": m}))
		s.t.Fatalf("candidate admission blocked: %v", err)
	}
	s.Must(controlled.Write(filepath.Join(s.Output, "manifest.json"), m))
	return m
}

func (s *Scenario) NewCandidateProject(m native.CandidateManifest) *Project {
	profiles, err := m.Selection(s.ID)
	s.Must(err)
	// Current remote selection always fails the product permission preflight.
	// Never construct a local substitute if that admission is unavailable.
	for _, profile := range profiles {
		if profile.Chain == "orca-remote" {
			s.t.Fatal("remote adapter cannot run without target permission receipts")
		}
	}
	p := s.NewLiveProject("candidates", "orca-codex")
	raw, err := os.ReadFile(filepath.Join(p.Home, "execution.json"))
	s.Must(err)
	var config struct {
		Version  int              `json:"version"`
		Profiles []map[string]any `json:"execution_profiles"`
		Default  string           `json:"default_execution_profile"`
	}
	s.Must(json.Unmarshal(raw, &config))
	orca := config.Profiles[0]
	codex, err := native.CodexExecutable("codex")
	s.Must(err)
	config.Profiles = nil
	for _, profile := range profiles {
		if profile.Chain == "orca-codex" {
			orca["id"], orca["name"] = profile.ID, "Candidate local Orca"
			config.Profiles = append(config.Profiles, orca)
		} else {
			config.Profiles = append(config.Profiles, map[string]any{"id": profile.ID, "name": "Candidate local Codex", "host": "local", "agent": "codex", "implementation_configs": native.WorkerImplementationConfigs(codex, nil)})
		}
	}
	config.Default = profiles[0].ID
	s.Must(controlled.Write(filepath.Join(p.Home, "execution.json"), config))
	spec := `# Candidate native fixture

On the INITIAL business turn only, read this Spec/Plan and the assigned branch, execute powershell.exe -NoProfile -Command "Start-Sleep -Seconds 20" once to allow the sibling candidate to start, then end the turn with a distinct readiness summary including that branch. Do not change files, publish, ask a native question or invoke another model. The operator will send feedback to this original session through SteerLine.

On each subsequent feedback turn, follow its exact marker. Write result.txt containing exactly real model delivery followed by a newline. Write candidate.txt containing exactly the feedback marker followed by a newline. Verify both. Run powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./publish.ps1 once. Publication belongs to you: the script commits, pushes the assigned branch, creates its real GitHub draft PR or updates the same branch's existing PR, and prints its URL. Include the full URL in the final reply/completion relay. Never push the base branch. Keep all other tracked files unchanged. Do not call another model.

## Acceptance
The operator compares independent candidate markers and merges exactly one PR. Preserve the original session across feedback. A successor follows this same contract from its new merged base with a new marker.
`
	s.Must(os.WriteFile(filepath.Join(p.Repo, "docs", "spec", "task.spec.md"), []byte(spec), 0600))
	s.Must(os.WriteFile(filepath.Join(p.Repo, "docs", "spec", "task.plan.md"), []byte("# Candidate fixture plan\n\nEnd the initial turn after reading the contract. Resume the original session only with operator feedback. Write and verify result.txt/candidate.txt and publish using the supplied script.\n\n## Test plan\n\nVerify both exact file contents before worker commit/push/create. The operator checks independent IDs, native usage, feedback, real PR head and merged successor base.\n"), 0600))
	raw, err = os.ReadFile(filepath.Join(p.Repo, "publish.ps1"))
	s.Must(err)
	script := strings.Replace(string(raw), "git add -- result.txt", "git add -- result.txt candidate.txt", 1)
	create := "gh pr create --repo"
	start := strings.Index(script, create)
	if start < 0 {
		s.t.Fatal("worker publish script missing GitHub creation")
	}
	lineEnd := strings.Index(script[start:], "\n")
	if lineEnd < 0 {
		s.t.Fatal("worker publication line missing")
	}
	lineEnd += start
	creation := script[start:lineEnd]
	script = script[:start] + fmt.Sprintf("$existing = gh pr list --repo '%s' --base '%s' --head $branch --state open --json url --jq '.[0].url'\nif ($LASTEXITCODE -ne 0) { throw 'PR lookup failed' }\nif ($existing) { Write-Output $existing } else { %s }", githubRepository, p.BaseBranch, creation) + script[lineEnd:]
	s.Must(os.WriteFile(filepath.Join(p.Repo, "publish.ps1"), []byte(script), 0600))
	p.git(p.Repo, "add", ".")
	p.git(p.Repo, "commit", "--quiet", "-m", "Pin native candidate feedback fixture")
	p.git(p.Repo, "push", "--quiet", "origin", p.BaseBranch)
	return p
}

func requestID() string { return strings.ReplaceAll(uuid.NewString(), "-", "") }

func (p *Project) CandidateControl(in driver.CandidateCommand) driver.CandidateReceipt {
	if in.RequestID == "" {
		in.RequestID = requestID()
	}
	if in.Action == "start_candidate" || in.Action == "abandon_candidate" {
		snap := mustSnapshot(p)
		for _, t := range snap.Page.Tasks {
			if t.TaskID == in.TaskID {
				in.TaskVersion = t.Version
			}
		}
		if in.Action == "start_candidate" {
			in.GraphRevision = snap.Page.GraphRevision
		} else {
			if snap.Runtime != nil {
				for _, run := range snap.Runtime.Runs {
					if run.RunID == in.RunID {
						in.CandidateVersion = run.ControlRevision
					}
				}
			}
		}
		if snap.Runtime != nil {
			in.InstanceID = snap.Runtime.Service.InstanceID
		}
	}
	p.Scenario.Must(controlled.Write(filepath.Join(p.Scenario.Output, "intent-"+in.RequestID+".json"), in))
	result, err := p.Driver.Candidate(p.Scenario.Context, p.Scenario.Run.Helper, in)
	p.Scenario.Must(controlled.Write(filepath.Join(p.Scenario.Output, "request-"+in.RequestID+".json"), map[string]any{"input": in, "receipt": result, "error": fmt.Sprint(err)}))
	p.Scenario.Must(err)
	return result
}
func (p *Project) StartCandidate(key, profile string, first bool) Candidate {
	action := "start_candidate"
	if first {
		action = "start_task"
	}
	receipt := p.CandidateControl(driver.CandidateCommand{Action: action, TaskID: p.Tasks[key].ID, ProfileID: profile})
	if receipt.Control == nil || receipt.Control.Status != "applied" {
		p.Scenario.t.Fatalf("candidate start: %+v", receipt)
	}
	var result Candidate
	p.Scenario.Wait("exact candidate execution", func() (bool, error) {
		snap, err := p.Snapshot()
		if err != nil {
			return false, err
		}
		for _, e := range snap.Domain.Executions {
			if e.WorkflowRunID == receipt.Control.RunID {
				result = Candidate{RunID: e.WorkflowRunID, ExecutionID: e.ExecutionID, ProfileID: profile}
				return e.TurnNumber > 0, nil
			}
		}
		return false, nil
	})
	return result
}
func (p *Project) CandidateSnapshot(c Candidate) Snapshot {
	snap, err := p.Snapshot()
	p.Scenario.Must(err)
	selected, err := evidence.Candidate(snap.Domain, c.RunID, c.ExecutionID)
	p.Scenario.Must(err)
	if selected.WorkflowRuns[0].ExecutionProfile.ID != c.ProfileID {
		p.Scenario.t.Fatal("candidate frozen profile changed")
	}
	snap.Domain = selected
	return snap
}
func (p *Project) WaitCandidate(c Candidate, phase string) Snapshot {
	var result Snapshot
	p.Scenario.Wait("candidate "+c.RunID+" "+phase, func() (bool, error) {
		result = p.CandidateSnapshot(c)
		e := result.Domain.Executions[0]
		p.Record(c.RunID+"-latest", result)
		if e.Environment != nil && e.Environment.Blocker != nil {
			return false, fmt.Errorf("original candidate environment blocker %s (no automatic feedback replay)", e.Environment.Blocker.Fact.Reason)
		}
		return result.Domain.WorkflowRuns[0].Phase == phase && e.Status != "active", nil
	})
	return result
}
func (p *Project) Feedback(c Candidate, marker string) driver.CandidateCommand {
	snap := p.CandidateSnapshot(c)
	var decision *contracts.DecisionState
	var revision uint64
	if snap.Runtime != nil {
		for _, r := range snap.Runtime.Runs {
			if r.RunID == c.RunID {
				decision = r.Decision
				revision = r.ControlRevision
			}
		}
	}
	if decision == nil || decision.Status != "pending" {
		p.Scenario.t.Fatal("original candidate has no pending decision")
	}
	if len(decision.Request.Options) == 0 || decision.Request.Options[0].ID != "continue" || !decision.Request.AllowFreeText {
		p.Scenario.t.Fatal("complete option/free text feedback missing")
	}
	in := driver.CandidateCommand{Action: "answer_decision", RequestID: requestID(), RunID: c.RunID, ExecutionID: c.ExecutionID, DecisionID: decision.ID, DecisionVersion: decision.Version, RunRevision: revision, InstanceID: snap.Runtime.Service.InstanceID, Option: "continue", Note: "Use exactly this candidate.txt marker: " + marker + ". Publish according to the bound Spec/Plan in this original session."}
	r := p.CandidateControl(in)
	if r.Command == nil {
		p.Scenario.t.Fatal("original feedback command missing")
	}
	return in
}
func (p *Project) CandidatePR(c Candidate) contracts.PRInfo {
	snap := p.CandidateSnapshot(c)
	info, err := p.githubPR(snap)
	p.Scenario.Must(err)
	e := snap.Domain.Executions[0]
	if info.HeadBranch != e.Workspace.Branch || info.HeadRepository != githubRepository || info.HeadRevision != p.git(e.Workspace.Worktree, "rev-parse", "HEAD") {
		p.Scenario.t.Fatal("candidate worker PR head/identity mismatch")
	}
	p.Scenario.Must(controlled.Write(filepath.Join(p.Scenario.Output, c.RunID+"-pr.json"), info))
	return info
}
func (p *Project) MergeCandidate(c Candidate) contracts.PRInfo {
	info := p.CandidatePR(c)
	if info.State != "open" {
		p.Scenario.t.Fatal("operator needs an owned open PR")
	}
	_, err := p.gh(p.Scenario.Context, "pr", "ready", info.URL, "--repo", githubRepository)
	p.Scenario.Must(err)
	_, err = p.gh(p.Scenario.Context, "pr", "merge", info.URL, "--repo", githubRepository, "--merge", "--match-head-commit", info.HeadRevision)
	p.Scenario.Must(err)
	return info
}

func (p *Project) CandidateUsage(c Candidate, label string) native.Usage {
	var u native.Usage
	p.Scenario.Wait("candidate native usage", func() (bool, error) {
		var err error
		u, err = p.NativeUsage(p.CandidateSnapshot(c), c.RunID+"-"+label)
		if errors.Is(err, native.ErrNativeTurnPending) {
			return false, nil
		}
		return err == nil, err
	})
	p.Scenario.Must(evidence.FullAccess(p.CandidateSnapshot(c).Domain))
	snap := p.CandidateSnapshot(c)
	canonicalDeadline := time.Now().Add(45 * time.Second)
	p.Scenario.Wait("canonical original candidate final/body", func() (bool, error) {
		conversation, err := driver.ReadCandidateConversation(p.Scenario.Context, p.Database, snap.Page.ProjectID, c.RunID, c.ExecutionID)
		if err != nil {
			return false, err
		}
		p.Scenario.Must(controlled.Write(filepath.Join(p.Scenario.Output, c.RunID+"-"+label+"-conversation.json"), conversation))
		finals := map[string]bool{}
		for _, event := range conversation.Events {
			if event.Type == contracts.ConversationMessageComplete && event.Phase == contracts.MessagePhaseFinalAnswer && event.SourceRole == contracts.ConversationSourceCanonical {
				if event.SessionID != u.Session {
					return false, errors.New("canonical final belongs to another native session")
				}
				finals[event.TurnID] = true
			}
		}
		expected := int(snap.Domain.Executions[0].TurnNumber)
		if snap.Domain.Executions[0].Backend == "orca" {
			expected++
		}
		if len(finals) >= expected && len(conversation.Bodies) > 0 {
			return true, nil
		}
		if time.Now().After(canonicalDeadline) {
			p.Scenario.t.Fatalf("canonical original candidate final/body missing: native turns=%d expected=%d; see conversation.json", len(finals), expected)
		}
		return false, nil
	})
	return u
}

// CandidateAcceptance retains full product snapshots and exact per-candidate
// receipts. It never selects a worker by slice order or performs worker publication.
func CandidateAcceptance(s *Scenario) {
	m := s.CandidateManifest()
	profiles, err := m.Selection(s.ID)
	s.Must(err)
	p := s.NewCandidateProject(m)
	p.CreateTasks()
	p.StartService()
	s.Must(p.Driver.Control(s.Context, s.Run.Helper, "resume_dispatch", ""))
	a := p.StartCandidate("a", profiles[0].ID, true)
	b := p.StartCandidate("a", profiles[1].ID, false)
	aw := p.WaitCandidate(a, "waiting_decision")
	bw := p.WaitCandidate(b, "waiting_decision")
	ae, be := aw.Domain.Executions[0], bw.Domain.Executions[0]
	for _, e := range []contracts.ExecutionRecord{ae, be} {
		if e.Backend == "orca" {
			s.Must(evidence.OrcaReadiness(e))
		}
	}
	as, err := ExecutionSession(ae)
	s.Must(err)
	bs, err := ExecutionSession(be)
	s.Must(err)
	if as == bs || ae.Workspace.Worktree == be.Workspace.Worktree || ae.Workspace.Branch == be.Workspace.Branch || ae.Workspace.Base != be.Workspace.Base {
		s.t.Fatal("candidate workspace/session/base isolation missing")
	}
	if ae.TurnStartedAt == nil || be.TurnStartedAt == nil || ae.TurnEndedAt == nil || be.TurnEndedAt == nil || !ae.TurnStartedAt.Before(*be.TurnEndedAt) || !be.TurnStartedAt.Before(*ae.TurnEndedAt) {
		s.t.Fatal("real candidate business turns did not overlap")
	}
	p.Record("both-waiting", mustSnapshot(p))
	ua, ub := p.CandidateUsage(a, "initial"), p.CandidateUsage(b, "initial")
	p.RestartService()
	p.WaitCandidate(a, "waiting_decision")
	p.WaitCandidate(b, "waiting_decision")
	if ua != p.CandidateUsage(a, "restart") || ub != p.CandidateUsage(b, "restart") {
		s.t.Fatal("restart repeated native model work")
	}
	for _, c := range []Candidate{a, b} {
		e := p.CandidateSnapshot(c).Domain.Executions[0]
		session, err := ExecutionSession(e)
		s.Must(err)
		original := as
		if c == b {
			original = bs
		}
		if session != original || e.TurnNumber != 1 {
			s.t.Fatal("restart replaced original worker")
		}
	}
	fa := p.Feedback(a, "alpha")
	p.Feedback(b, "bravo")
	p.WaitCandidate(a, "waiting_pr_merge")
	p.WaitCandidate(b, "waiting_pr_merge")
	// A caller that lost its answer response reuses the original command ID/content.
	replay := p.CandidateControl(fa)
	if replay.Command == nil || replay.Command.CommandID != fa.RequestID {
		s.t.Fatal("feedback replay changed original command")
	}
	pa, pb := p.CandidatePR(a), p.CandidatePR(b)
	published := map[Candidate]native.Usage{}
	if pa.URL == pb.URL {
		s.t.Fatal("candidate PRs are not distinct")
	}
	for _, pair := range []struct {
		c               Candidate
		marker, session string
	}{{a, "alpha", as}, {b, "bravo", bs}} {
		snap := p.CandidateSnapshot(pair.c)
		e := snap.Domain.Executions[0]
		session, err := ExecutionSession(e)
		s.Must(err)
		if session != pair.session || e.TurnNumber != 2 || p.git(e.Workspace.Worktree, "show", "HEAD:candidate.txt") != pair.marker {
			s.t.Fatal("feedback crossed candidate/session/turn")
		}
		published[pair.c] = p.CandidateUsage(pair.c, "published")
	}
	snap := mustSnapshot(p)
	for _, t := range snap.Page.Tasks {
		if t.TaskID == p.Tasks["b"].ID && (t.DependencyReady || len(t.WorkflowRuns) > 0) {
			s.t.Fatal("successor started before delivery")
		}
	}
	p.MergeCandidate(a)
	p.WaitCandidate(a, "completed")
	done := mustSnapshot(p)
	p.Record("selected-delivery", done)
	if len(done.Domain.Deliveries) != 1 || done.Domain.Deliveries[0].SelectedWorkflowRunID != a.RunID {
		s.t.Fatal("unique winning delivery missing")
	}
	merge := done.Domain.Deliveries[0].MergeCommit
	// Submit the actual post-merge command; a hidden UI button is no evidence.
	rejected := p.CandidateControl(driver.CandidateCommand{Action: "abandon_candidate", TaskID: p.Tasks["a"].ID, RunID: a.RunID, Note: "CN8 actual merged candidate rejection"})
	if rejected.Control == nil || rejected.Control.Status != "rejected" {
		s.t.Fatal("merged candidate abandonment was accepted")
	}
	loser := p.CandidateSnapshot(b)
	if loser.Domain.WorkflowRuns[0].PRURL != pb.URL {
		s.t.Fatal("losing original PR attribution lost")
	}
	p.Record("loser-resource-facts", loser)
	next := p.StartCandidate("b", profiles[0].ID, true)
	ns := p.WaitCandidate(next, "waiting_decision")
	ne := ns.Domain.Executions[0]
	p.git(ne.Workspace.Worktree, "merge-base", "--is-ancestor", merge, ne.Workspace.Base)
	p.Feedback(next, "successor")
	p.WaitCandidate(next, "waiting_pr_merge")
	published[next] = p.CandidateUsage(next, "published")
	p.MergeCandidate(next)
	p.WaitCandidate(next, "completed")
	final := mustSnapshot(p)
	p.Record("final-deliveries", final)
	if len(final.Domain.Deliveries) != 2 {
		s.t.Fatal("successor real delivery missing or duplicate")
	}
	for i := 0; i < 3; i++ {
		if len(mustSnapshot(p).Domain.Deliveries) != 2 {
			s.t.Fatal("polling duplicated delivery")
		}
	}
	for c, baseline := range published {
		if baseline != p.CandidateUsage(c, "final") {
			s.t.Fatal("delivery/polling replaced native session/turn or consumed usage")
		}
	}
	proof := p.CandidatePR(a)
	p.RealPRProof = &proof
	s.usageVerified = true
	p.StopService()
}
func mustSnapshot(p *Project) Snapshot { snap, err := p.Snapshot(); p.Scenario.Must(err); return snap }
