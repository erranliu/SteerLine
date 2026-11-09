package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/mainagent"
	ma "github.com/erranliu/SteerLine/internal/mainagent/model"
	"github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/httpapi"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

type mainAgentAnswerTestOwner struct {
	httpapi.QueryPort
	httpapi.CommandPort
	httpapi.AuthorityPort
	httpapi.NativeSessionPort
	identity  string
	candidate *api.CandidateSummary
	received  *api.DecisionInput
	receipt   api.CommandReceipt
}

func (o *mainAgentAnswerTestOwner) Project() api.ProjectRef {
	return api.ProjectRef{Identity: o.identity}
}
func (o *mainAgentAnswerTestOwner) ReadProject(context.Context, string, api.ProjectSnapshot) api.ProjectSnapshot {
	return api.ProjectSnapshot{Connection: api.ServiceConnection{Snapshot: api.ObservationSnapshot{Page: &api.ServicePage{Runs: []*api.CandidateSummary{o.candidate}}}}}
}
func (o *mainAgentAnswerTestOwner) Command(context.Context, string) (api.CommandReceipt, error) {
	if o.received == nil {
		return api.CommandReceipt{}, &api.Error{Kind: api.UnknownRecord, Message: "not submitted"}
	}
	return o.receipt, nil
}
func (o *mainAgentAnswerTestOwner) NormalizeDecision(_ context.Context, in api.DecisionInput) (api.DecisionInput, error) {
	return in, nil
}
func (o *mainAgentAnswerTestOwner) Request(context.Context, string) (*api.ControlRequest, error) {
	return nil, os.ErrNotExist
}
func (o *mainAgentAnswerTestOwner) SubmitDecision(_ context.Context, in api.DecisionInput) (api.CommandReceipt, bool, error) {
	o.received = &in
	o.receipt = api.CommandReceipt{ProjectIdentity: o.identity, CommandID: in.CommandID, Kind: "answer_decision", TargetID: in.Answer.DecisionID, ExpectedVersion: in.Answer.ExpectedDecisionVersion, RequestedBy: in.RequestedBy, DecisionAnswer: &in.Answer, Status: "accepted"}
	return o.receipt, false, nil
}

func TestMainAgentAnswerUsesOriginalHTTPDecisionOwner(t *testing.T) {
	for _, stale := range []string{"current", "attempt", "recovery"} {
		t.Run(stale, func(t *testing.T) {
			p, r, _ := materialPublicationFixture(t)
			r.ID, r.Revision = strings.Repeat("c", 32), 1
			r.Publication = nil
			r.Context.CandidateID = strings.Repeat("e", 32)
			r.Context.CandidateRevision, r.Context.RunnerRevision = 4, 7
			r.Context.SourceAttemptID = "original-attempt"
			r.Context.Question = &runnercontract.Question{ID: "original-question", AllowFreeText: true}
			r.Result = &ma.Result{Disposition: "clarification", Answer: &ma.Answer{Note: "Original answer"}, Summary: "Explain existing text", Recommendation: "Answer original question", Questions: []string{}, Changes: []ma.Change{}}
			require(t, p.store.SaveAnalysis(context.Background(), r, 0))
			owner := &mainAgentAnswerTestOwner{identity: p.config.Identity(), candidate: &api.CandidateSummary{RunID: r.Context.CandidateID, ControlRevision: 4, AttemptID: "original-attempt", RecoveryVersion: 7, Decision: &api.DecisionSummary{ID: "original-question", Version: 7}}}
			if stale == "attempt" {
				owner.candidate.AttemptID = "replacement-attempt"
			}
			if stale == "recovery" {
				owner.candidate.RecoveryVersion++
			}
			server, err := httpapi.NewProject(httpapi.Ports{Query: owner, Commands: owner, Authority: owner, NativeViewer: owner}, httpapi.Options{RunDir: t.TempDir(), ProjectIdentity: owner.identity, InstanceID: "instance", Capabilities: []string{"answer_decision"}})
			require(t, err)
			require(t, server.SetReady("instance", true, ""))
			c := p.config
			c.runtimeMainAgent = p
			runtime := &runtimeProjectPort{Server: server, config: c, store: p.store}
			body, _ := json.Marshal(map[string]string{"review": ma.Review(r)})
			_, err = runtime.applyMainAgentAnswer(context.Background(), httpcontract.CommandEnvelope{RequestID: strings.Repeat("d", 32), ProjectID: httpcontract.ProjectID(c.Identity()), InstanceID: "instance", TargetID: r.ID, ExpectedVersion: r.Revision, Payload: body})
			if stale != "current" {
				if err == nil || owner.received != nil {
					t.Fatal("stale original attempt/recovery reached answer owner", err, owner.received)
				}
				return
			}
			require(t, err)
			if owner.received == nil || owner.received.Answer.SourceAttemptID != "original-attempt" || owner.received.Answer.ExpectedRecoveryVersion != 7 || owner.received.Answer.Note != "Original answer" {
				t.Fatal("reviewed original answer was not delivered", owner.received)
			}
		})
	}
}

func materialPublicationFixture(t *testing.T) (*mainAgentPort, ma.Record, string) {
	t.Helper()
	ctx := context.Background()
	c, _ := nativeTaskFixture(t)
	fixtureGit(t, c.Repository, "config", "user.name", "Material fixture")
	fixtureGit(t, c.Repository, "config", "user.email", "material@example.invalid")
	fixtureGit(t, c.Repository, "config", "commit.gpgsign", "false")
	require(t, os.WriteFile(filepath.Join(c.Repository, "task.spec.md"), []byte("# Spec\nOriginal requirement.\n"), 0600))
	require(t, os.WriteFile(filepath.Join(c.Repository, "task.plan.md"), []byte("# Plan\nVerify requirement.\n"), 0600))
	require(t, mainagent.PrepareRules(c.Repository, false))
	fixtureGit(t, c.Repository, "add", ".")
	fixtureGit(t, c.Repository, "commit", "-qm", "task materials")
	base := fixtureGit(t, c.Repository, "rev-parse", "HEAD")
	bare := filepath.Join(t.TempDir(), "origin.git")
	fixtureGit(t, c.Repository, "clone", "--bare", c.Repository, bare)
	// Keep the provider-facing repository identity while Git uses a private bare fixture.
	fixtureGit(t, c.Repository, "remote", "set-url", "origin", "https://github.com/owner/repo.git")
	fixtureGit(t, c.Repository, "config", "url."+filepath.ToSlash(bare)+".insteadOf", "https://github.com/owner/repo.git")
	reader, err := OpenNativeTaskProject(ctx, &c)
	require(t, err)
	require(t, reader.Close())
	s, err := OpenNativeTaskCommandStore(ctx, &c)
	require(t, err)
	t.Cleanup(func() { require(t, s.Close()) })
	c.runtimeMaintenance = newProjectMaintenance(c, s)
	p := &mainAgentPort{config: c, store: s}
	p.service = &mainagent.Service{Project: c.Identity(), Store: s}
	before, err := c.runtimeMaintenance.read(ctx, base, "task.spec.md")
	require(t, err)
	doc := ma.Document{Path: "task.spec.md", Commit: base, SHA256: ma.Digest(before), Body: string(before)}
	r := ma.Record{Project: c.Identity(), ID: strings.Repeat("a", 32), Revision: 1, CreatedAt: time.Now().UTC(), Context: ma.Context{TaskID: "1", SpecPath: doc.Path, PlanPath: "task.plan.md", RepositoryCommit: base, OriginalBase: base, Original: []ma.Document{doc}, Current: []ma.Document{doc}}, Input: ma.Start{Kind: "conflict", Note: "Conflicting requirement"}, State: "review", Submission: "started", Projection: &runnercontract.Run{StopStatus: "stopped"}, Result: &ma.Result{Disposition: "revision", Summary: "Clarify existing requirement", Recommendation: "Update Spec and verify", Evidence: []ma.Evidence{{Path: doc.Path, Commit: base, Quote: "Original requirement."}}, Questions: []string{}, Changes: []ma.Change{{Path: doc.Path, BeforeSHA256: doc.SHA256, Body: "# Spec\nClarified requirement.\n", Reason: "Resolve contradiction"}}}}
	r.Publication = &ma.Publication{RequestID: strings.Repeat("b", 32), Review: ma.Review(r), Base: base, Branch: "codex/material-analysis-" + r.ID, State: "preparing"}
	r.Request = runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: "analysis-original", Input: json.RawMessage(`{}`)}
	require(t, s.SaveAnalysis(ctx, r, 0))
	return p, r, bare
}
func TestMainAgentPublicationContainsOnlyReviewedDocuments(t *testing.T) {
	for _, hook := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "hook-adds-code"}[hook], func(t *testing.T) {
			p, r, _ := materialPublicationFixture(t)
			if hook {
				hooks := t.TempDir()
				require(t, os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\necho unexpected > injected.txt\ngit add injected.txt\n"), 0700))
				fixtureGit(t, p.config.Repository, "config", "core.hooksPath", filepath.ToSlash(hooks))
			}
			err := p.preparePublication(context.Background(), &r)
			if hook {
				if err == nil || !strings.Contains(err.Error(), "未审阅") {
					t.Fatal("hook smuggled code into publication", err)
				}
				return
			}
			require(t, err)
			changed := fixtureGit(t, p.config.Repository, "diff", "--name-only", r.Publication.Base, r.Publication.Commit)
			if changed != "task.spec.md" {
				t.Fatal(changed)
			}
			if fixtureGit(t, p.config.Repository, "rev-parse", "HEAD") != r.Publication.Base {
				t.Fatal("main workspace was changed")
			}
			body, err := os.ReadFile(filepath.Join(p.config.Repository, "task.spec.md"))
			require(t, err)
			if string(body) != r.Context.Current[0].Body {
				t.Fatal("source document overwritten")
			}
		})
	}
}
func TestMainAgentLostPRReceiptQueriesOriginalPublication(t *testing.T) {
	p, r, _ := materialPublicationFixture(t)
	ctx := context.Background()
	require(t, p.preparePublication(ctx, &r))
	fixtureGit(t, p.config.Repository, "push", "origin", r.Publication.Commit+":refs/heads/"+r.Publication.Branch)
	r.Publication.State = "pr_unknown"
	r, err := p.service.Save(ctx, r)
	require(t, err)
	posts := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		if q.Method != "GET" {
			posts++
			http.Error(w, "no writes allowed", 500)
			return
		}
		fact := roadmapPR{URL: server.URL + "/owner/repo/pulls/1", Number: 1, State: "open"}
		fact.Head.Ref = r.Publication.Branch
		fact.Head.SHA = r.Publication.Commit
		fact.Head.Repo.FullName = "owner/repo"
		fact.Base.Ref = p.config.Branch
		_ = json.NewEncoder(w).Encode([]roadmapPR{fact})
	}))
	defer server.Close()
	p.config.PR = executionmodel.PRProviderConfig{Kind: "gitea", BaseURL: server.URL, Repository: "owner/repo"}
	c := p.config
	c.runtimeMainAgent = p
	runtime := &runtimeProjectPort{config: c, store: p.store}
	in := httpcontract.CommandEnvelope{RequestID: r.Publication.RequestID, ProjectID: httpcontract.ProjectID(c.Identity()), InstanceID: "instance", Action: "main_agent_publish", TargetID: r.ID, ExpectedVersion: 1, Payload: json.RawMessage(`{}`)}
	raw, _ := json.Marshal(in)
	out := maintenanceResponse(in)
	out.Status = "outcome_unknown"
	receipt, _ := json.Marshal(out)
	_, _, err = p.store.EnqueueCommand(ctx, ports.CommandRecord{ProjectIdentity: c.Identity(), CommandID: in.RequestID, IdempotencyKey: in.RequestID, Kind: "main_agent_command", TargetID: r.ID, ExpectedVersion: 1, RequestedBy: "test", Payload: raw, Result: receipt, Status: "outcome_unknown"})
	require(t, err)
	got, err := runtime.MainAgentReceipt(ctx, in.RequestID)
	require(t, err)
	if got == nil || got.Status != "applied" || posts != 0 {
		t.Fatal("original publication could not be reconciled without writes", got, posts)
	}
	stored, err := p.store.Analysis(ctx, c.Identity(), r.ID)
	require(t, err)
	if stored.Publication.State != "published" || stored.Publication.URL == "" {
		t.Fatal(stored.Publication)
	}
}
func TestMainAgentAnswerRejectsChangedMaterialBaseline(t *testing.T) {
	p, r, _ := materialPublicationFixture(t)
	r.Publication = nil
	r.Result.Changes = nil
	r.Result.Disposition = "clarification"
	r.Result.Answer = &ma.Answer{Note: "Original answer"}
	r.Context.Question = &runnercontract.Question{ID: "original-question", AllowFreeText: true}
	// Use a new immutable analysis rather than mutate a saved publication record.
	r.ID = strings.Repeat("c", 32)
	r.Revision = 1
	require(t, p.store.SaveAnalysis(context.Background(), r, 0))
	require(t, os.WriteFile(filepath.Join(p.config.Repository, "task.plan.md"), []byte("# Changed Plan\n"), 0600))
	fixtureGit(t, p.config.Repository, "add", ".")
	fixtureGit(t, p.config.Repository, "commit", "-qm", "new plan")
	fixtureGit(t, p.config.Repository, "push", "origin", "main")
	c := p.config
	c.runtimeMainAgent = p
	runtime := &runtimeProjectPort{config: c, store: p.store}
	body, _ := json.Marshal(map[string]string{"review": ma.Review(r)})
	_, err := runtime.applyMainAgentAnswer(context.Background(), httpcontract.CommandEnvelope{RequestID: strings.Repeat("d", 32), TargetID: r.ID, ExpectedVersion: r.Revision, Payload: body})
	if err == nil || !strings.Contains(err.Error(), "基线已变化") {
		t.Fatal("stale material answer reached input owner", err)
	}
	stored, err := p.store.Analysis(context.Background(), c.Identity(), r.ID)
	require(t, err)
	if stored.AnswerAction != nil {
		t.Fatal("stale answer was reserved")
	}
}
func TestMainAgentStartupNeverResubmitsUnsentAnalysis(t *testing.T) {
	for _, cold := range []bool{true, false} {
		t.Run(map[bool]string{true: "cold-proven-absent", false: "online-unknown"}[cold], func(t *testing.T) {
			cli := startupFixtureCLI(t)
			if !cold {
				cli.ResourceDigest = strings.Repeat("a", 64)
				require(t, os.WriteFile(filepath.Join(cli.StateDir, "idle-fixture.json"), mustJSON(runnerclient.ServiceInfo{Protocol: runnerclient.Protocol, StateDir: cli.StateDir, Status: "ready", InstanceID: "original", Version: cli.BuildVersion, ResourceDigest: cli.ResourceDigest, ExecutableSHA256: cli.SHA256}), 0600))
			}
			require(t, os.WriteFile(filepath.Join(cli.StateDir, "idle-run-fixture.json"), []byte(`{"fixture_error":"request_not_found"}`), 0600))
			s, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
			require(t, err)
			defer s.Close()
			c := Config{identity: "project"}
			r := ma.Record{Project: c.Identity(), ID: strings.Repeat("a", 32), Revision: 1, CreatedAt: time.Now().UTC(), Runner: cli.BuildPin(), State: "unknown", Submission: "started", Request: runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: "original-analysis"}}
			if r.Project == "" {
				c.Repository = filepath.Dir(cli.StateDir)
				require(t, c.ResolveIdentity(context.Background()))
				r.Project = c.Identity()
			}
			require(t, s.SaveAnalysis(context.Background(), r, 0))
			stops, err := prepareMainAgentStops(context.Background(), &c, s, &cli)
			if cold {
				require(t, err)
				got, e := s.Analysis(context.Background(), r.Project, r.ID)
				require(t, e)
				if !got.Settled() || got.Submission != "rejected" {
					t.Fatal(got)
				}
			} else if err == nil {
				t.Fatal("online missing request treated as unsent")
			}
			if len(stops) > 0 {
				t.Fatal("missing analysis created stop or new input")
			}
			if _, err = os.Stat(filepath.Join(cli.StateDir, "business-dispatch.log")); !os.IsNotExist(err) {
				t.Fatal("startup sent new model input")
			}
		})
	}
}
