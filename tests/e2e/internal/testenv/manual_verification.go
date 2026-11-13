package testenv

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	mv "github.com/erranliu/SteerLine/internal/manualverification"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/google/uuid"
)

const ManualVerificationLiveID = "manual-verification.local-codex"

func ManualVerificationLive(s *Scenario) {
	p := s.NewLiveProject("manual-verification", "local-codex")
	p.CreateTasks()
	p.StartService()
	p.Claim("a")
	open := p.WaitPhase("waiting_pr_merge")
	p.AssertRunnerBinding(open)
	c := open.Runner.Candidates[0]
	if c.Projection.PR == nil || !c.Projection.PR.InitialDraftVerified || open.Helper.PR.Draft == nil || !*open.Helper.PR.Draft {
		s.t.Fatal("Runner and GitHub did not confirm the initial draft PR")
	}
	p.Record("draft-open", open)
	project, err := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
	s.Must(err)
	identity := project.Identity()
	project.Close()
	client, err := applicationclient.Open(s.Context, p.Home)
	s.Must(err)
	defer client.Close()
	prefix := "/projects/" + identity.ProjectID
	wait := func(accept func(mv.Record) bool) mv.Record {
		for s.Context.Err() == nil {
			var records []mv.Record
			s.Must(client.Get(s.Context, prefix+"/manual-verification", &records))
			for _, r := range records {
				if r.Candidate == c.ID && accept(r) {
					return r
				}
			}
			time.Sleep(400 * time.Millisecond)
		}
		s.t.Fatal("timed out waiting for the original manual verification record")
		return mv.Record{}
	}
	post := func(action string, r mv.Record, payload map[string]any) httpcontract.CommandResponse {
		body, err := json.Marshal(payload)
		s.Must(err)
		request := httpcontract.CommandEnvelope{RequestID: strings.ReplaceAll(uuid.NewString(), "-", ""), ProjectID: identity.ProjectID, InstanceID: identity.InstanceID, Action: "manual_verification_" + action, TargetID: r.Candidate, ExpectedVersion: r.Revision, Payload: body}
		var response httpcontract.CommandResponse
		s.Must(client.Post(s.Context, prefix+"/commands", request, &response))
		return response
	}
	applied := func(response httpcontract.CommandResponse) {
		if response.Status != "applied" {
			s.t.Fatalf("%s: %s", response.Action, response.Diagnostic)
		}
	}
	start := func(r mv.Record, source mv.Source, head string) (mv.Record, mv.Attempt) {
		previous := len(r.Attempts)
		applied(post("start", r, map[string]any{"head": r.Head, "source": source}))
		r = wait(func(value mv.Record) bool {
			if len(value.Attempts) <= previous {
				return false
			}
			a := value.Attempts[previous]
			if a.Status == "failed" {
				s.t.Fatal(a.Diagnostic)
			}
			return a.Status == "ready"
		})
		a := r.Attempts[previous]
		resp, err := (&http.Client{Timeout: 5 * time.Second}).Get(a.URL)
		s.Must(err)
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		s.Must(err)
		if strings.TrimSpace(string(body)) != "real model delivery" || a.Source != source || a.Head != head || p.git(a.Worktree, "rev-parse", "HEAD") != head {
			s.t.Fatal("preview did not run the exact selected head")
		}
		return r, a
	}
	r := wait(func(r mv.Record) bool { return r.Head == open.Helper.PR.HeadRevision })
	if len(r.Results) != 0 || len(r.Attempts) != 0 || !r.Draft {
		s.t.Fatal("draft publication waited on or fabricated manual verification")
	}
	if s.Run.Web == "" {
		s.t.Fatal("manual verification requires the frozen Web executable")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	s.Must(err)
	address := listener.Addr().String()
	s.Must(listener.Close())
	s.Must(controlled.Write(filepath.Join(p.Home, "web.json"), map[string]any{"version": 1, "address": address}))
	s.start(p.Command(s.Run.Web, "serve"), "manual-web")
	s.Wait("manual verification Web listener", func() (bool, error) {
		conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if err == nil {
			conn.Close()
		}
		return err == nil, err
	})
	node, err := exec.LookPath("node")
	s.Must(err)
	cli := filepath.Join(s.Run.Root, "web", "node_modules", "@playwright", "test", "cli.js")
	cmd := p.Command(node, cli, "test", "tests/manual-verification-live.spec.ts")
	cmd.Dir = filepath.Join(s.Run.Root, "web")
	cmd.Env = append(cmd.Env, "STEERLINE_MANUAL_WEB="+s.Run.Web, "STEERLINE_MANUAL_PROJECT="+identity.ProjectID, "STEERLINE_MANUAL_CANDIDATE="+c.ID, "STEERLINE_MANUAL_HEAD="+r.Head, "STEERLINE_WEB_EVIDENCE="+filepath.Join(s.Output, "browser"))
	s.Must(s.start(cmd, "manual-browser").Wait(s.Context))
	r = wait(func(r mv.Record) bool { return len(r.Results) == 1 })
	a := r.Attempts[0]
	if a.Status != "ready" || p.git(a.Worktree, "rev-parse", "HEAD") != r.Head || r.Results[0].Outcome != "failed" {
		s.t.Fatal("browser did not start and record the exact draft Head")
	}
	old := r
	p.git(p.Repo, "fetch", "origin", open.Helper.PR.HeadBranch)
	p.git(p.Repo, "checkout", "--detach", old.Head)
	s.Must(os.WriteFile(filepath.Join(p.Repo, "second.txt"), []byte("second head\n"), 0600))
	p.git(p.Repo, "add", "--", "second.txt")
	p.git(p.Repo, "commit", "-m", "Exercise manual re-verification")
	newHead := p.git(p.Repo, "rev-parse", "HEAD")
	p.git(p.Repo, "push", "origin", "HEAD:refs/heads/"+open.Helper.PR.HeadBranch)
	r = wait(func(r mv.Record) bool { return r.Head == newHead })
	if len(r.Results) != 1 || r.Results[0].Head != old.Head {
		s.t.Fatal("old results did not keep their original Head")
	}
	if response := post("result", r, map[string]any{"head": old.Head, "attempt_id": a.ID, "check_id": "M01", "outcome": "passed"}); response.Status != "rejected" {
		s.t.Fatal("stale Head was accepted")
	}
	r, b := start(r, mv.SourcePR, newHead)
	applied(post("result", r, map[string]any{"head": r.Head, "attempt_id": b.ID, "check_id": "M01", "outcome": "passed"}))
	r = wait(func(r mv.Record) bool { return len(r.Results) == 2 })
	clean := func(r mv.Record, attempt mv.Attempt) mv.Record {
		applied(post("stop", r, map[string]any{"head": attempt.Head, "attempt_id": attempt.ID}))
		r = wait(func(r mv.Record) bool {
			i, err := r.Attempt(attempt.ID)
			return err == nil && r.Attempts[i].Status == "stopped"
		})
		applied(post("remove", r, map[string]any{"head": attempt.Head, "attempt_id": attempt.ID}))
		return wait(func(r mv.Record) bool { i, err := r.Attempt(attempt.ID); return err == nil && r.Attempts[i].Removed })
	}
	for _, attempt := range []mv.Attempt{a, b} {
		r = clean(r, attempt)
	}
	p.Merge()
	done := p.WaitPhase("completed")
	p.AssertOneDelivery(done)
	p.Record("completed", done)
	r = wait(func(r mv.Record) bool { return r.Merged && !r.Archived && r.Closure == nil && len(r.Results) == 2 })
	r, pr := start(r, mv.SourcePR, newHead)
	r = clean(r, pr)
	remoteHead := strings.Fields(p.git(p.Repo, "ls-remote", "origin", "refs/heads/"+open.Helper.PR.BaseBranch))[0]
	r, remote := start(r, mv.SourceRemote, remoteHead)
	if post("close", r, map[string]any{"head": remote.Head, "source": remote.Source}).Status != "rejected" {
		s.t.Fatal("the PR result was reused to close an unverified source")
	}
	applied(post("result", r, map[string]any{"head": remote.Head, "attempt_id": remote.ID, "check_id": "M01", "outcome": "failed", "notes": "Remote Head manual observation"}))
	r = wait(func(r mv.Record) bool { return len(r.Results) == 3 })
	applied(post("close", r, map[string]any{"head": remote.Head, "source": remote.Source}))
	closedAtRevision := r.Revision
	r = wait(func(r mv.Record) bool { return r.Revision > closedAtRevision })
	if r.Closure == nil {
		s.t.Fatal("applied close did not persist the verification closure")
	}
	applied(post("reopen", r, map[string]any{"head": remote.Head}))
	reopenedAtRevision := r.Revision
	r = wait(func(r mv.Record) bool { return r.Revision > reopenedAtRevision })
	if r.Closure != nil || len(r.Results) != 3 {
		s.t.Fatal("reopening did not preserve the original verification results")
	}
	applied(post("close", r, map[string]any{"head": remote.Head, "source": remote.Source}))
	r = wait(func(r mv.Record) bool { return r.Closure != nil })
	final := clean(r, remote)
	s.Must(controlled.Write(filepath.Join(s.Output, "manual-verification.json"), map[string]any{"initial_draft": *open.Helper.PR.Draft, "pr": r.PR, "first_head": old.Head, "second_head": newHead, "final": final, "delivery_count": len(done.Runner.Graph.Deliveries)}))
	s.Must(s.FinishBody())
	p.WaitStoppedCandidates()
	p.StopService()
}
