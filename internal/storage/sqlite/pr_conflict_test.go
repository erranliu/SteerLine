package sqlite

import (
	"context"
	"encoding/json"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/taskrunner"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPRConflictControlRetainsIntentAfterReceiptLoss(t *testing.T) {
	ctx := context.Background()
	s, project, tasks := nativeAuditStore(t)
	graph, err := s.TaskGraphSnapshot(ctx, project)
	require(t, err)
	record := graph.ByID[tasks[0].TaskID]
	contract, err := s.TaskContract(ctx, project, record)
	require(t, err)
	input := runnerclient.RunInput{Title: record.Title, Repository: runnerclient.Repository{URL: contract.Repository, Remote: contract.Remote, BaseBranch: contract.Branch, BaseRevision: strings.Repeat("a", 40)}, Documents: runnerclient.Documents{Spec: runnerclient.Document{Path: contract.SpecRef.Path}, Plan: runnerclient.Document{Path: contract.PlanRef.Path}}, Publication: runnerclient.Publication{BaseBranch: contract.Branch}}
	raw, _ := json.Marshal(input)
	candidate := runnerbridge.Candidate{Project: project, TaskID: record.TaskID, ID: strings.Repeat("c", 32), Revision: 1, Explicit: true, StateDir: filepath.Join(t.TempDir(), "runner"), Claimant: "fixture", ContractDigest: contract.Digest, BaseRevision: input.Repository.BaseRevision, Participation: "active", SubmitState: "pending", Submit: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "submit", Input: raw}}
	require(t, reserveFixtureCandidate(t, s, ctx, taskmodel.FixedWork{Record: record, Contract: contract, ConfirmedDigest: contract.Digest, Authority: graph.TaskGraph().Authority}, candidate, false))
	candidate, err = s.Candidate(ctx, project, candidate.ID)
	require(t, err)
	previous := candidate.Revision
	candidate.RunID, candidate.SubmitState, candidate.InputDigest = "original-run", "accepted", "original-input"
	candidate.Projection = &runnerclient.Run{RunID: candidate.RunID, InputDigest: candidate.InputDigest, Revision: 8, Status: "pr_ready", Controls: runnerclient.Controls{CanMessage: true}, PR: &runnerclient.PRFact{URL: "https://example.test/repo/pull/1", BaseBranch: "main", State: "open"}}
	candidate.Revision++
	require(t, s.SaveCandidate(ctx, candidate, previous, nil))
	bridge := &runnerbridge.Bridge{Store: s}
	controls := taskrunner.Controls{Tasks: &taskrunner.Service{Project: project, Store: s, Bridge: bridge}, Store: s, ResolveProfile: func(string, string) (string, error) {
		t.Fatal("conflict resolution consulted a new profile")
		return "", nil
	}}
	graph, err = s.TaskGraphSnapshot(ctx, project)
	require(t, err)
	now := time.Now().UTC()
	payload, _ := json.Marshal(api.ControlPayload{TaskID: record.TaskID, RunID: candidate.ID, ExpectedRunRevision: candidate.Revision})
	req := api.ControlRequest{ProtocolVersion: 1, RequestID: strings.Repeat("d", 32), RepositoryIdentity: project, InstanceID: "instance", Action: "resolve_pr_conflict", ExpectedControlRevision: graph.Authority.AuthorityVersion, RequestedBy: "operator", CreatedAt: now, ExpiresAt: now.Add(time.Minute), Payload: payload}
	_, err = s.db.ExecContext(ctx, `CREATE TRIGGER lose_conflict_receipt BEFORE UPDATE ON commands WHEN NEW.kind='runner_control' BEGIN SELECT RAISE(ABORT,'receipt fault'); END`)
	require(t, err)
	receipt, err := controls.Handle(ctx, req, "instance", graph.Authority.AuthorityVersion)
	if err == nil || receipt.Status != "applied" {
		t.Fatal("fixture did not lose the applied outer receipt", receipt, err)
	}
	fixed, err := s.Candidate(ctx, project, candidate.ID)
	require(t, err)
	if len(fixed.Actions) != 1 || fixed.Actions[0].Command != "message" || fixed.Actions[0].CallerRequestID != req.RequestID {
		t.Fatal("original message intent not persisted", fixed.Actions)
	}
	var body struct {
		Revision uint64 `json:"expected_revision"`
		Text     string `json:"text"`
	}
	require(t, json.Unmarshal(fixed.Actions[0].Request.Input, &body))
	if body.Revision != 8 || !strings.Contains(body.Text, "https://example.test/repo/pull/1") || !strings.Contains(body.Text, "目标分支：main") {
		t.Fatal("original facts lost", body)
	}
	_, err = s.db.ExecContext(ctx, `DROP TRIGGER lose_conflict_receipt`)
	require(t, err)
	replay, err := controls.Handle(ctx, req, "replacement", 99)
	require(t, err)
	if replay.Status != "applied" {
		t.Fatal("original receipt recovery failed", replay)
	}
	after, err := s.Candidate(ctx, project, candidate.ID)
	require(t, err)
	if len(after.Actions) != 1 || string(after.Actions[0].Request.Input) != string(fixed.Actions[0].Request.Input) {
		t.Fatal("receipt recovery rebuilt the message")
	}
}
