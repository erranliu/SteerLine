package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	taskapp "github.com/erranliu/SteerLine/internal/task"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

func TestRunnerControlRecoversCommittedStartAfterReceiptLoss(t *testing.T) {
	ctx := context.Background()
	s := openDomainTestStore(t)
	a := taskmodel.ProjectTaskAuthority{ProjectIdentity: "runner-control", Authority: "native", AuthorityVersion: 1, Repository: "https://example.invalid/Org/Repo.git", Remote: "origin", Branch: "main", DispatchMode: "manual", DispatchPaused: true}
	require(t, s.transactSQLite(ctx, func(tx *sqliteTx) error { return tx.PutProjectTaskAuthority(a) }))
	docs := &runnerTaskDocuments{repository: a.Repository, body: []byte("# Acceptance\nfixed contract\n")}
	sum := sha256.Sum256(docs.body)
	ref := taskmodel.FileReference{Path: "SteerlineFlow/spec/task.spec.md", Commit: strings.Repeat("a", 40), SHA256: hex.EncodeToString(sum[:])}
	tasks := nativeAuditRecordsWithReference(t, s, a, ref)
	require(t, s.transactSQLite(ctx, func(tx *sqliteTx) error {
		_, err := tx.tx.ExecContext(ctx, `UPDATE project_task_authorities SET dispatch_paused=0 WHERE project_identity=?`, a.ProjectIdentity)
		return err
	}))
	baseCalls := 0
	svc := &taskrunner.Service{Project: a.ProjectIdentity, Claimant: "operator", StateDir: filepath.Join(t.TempDir(), "runner"), DefaultProfile: "live", Store: s, Tasks: taskapp.Service{Reader: s, Documents: docs, Input: taskapp.Input{Project: a.ProjectIdentity, Authority: "native", Remote: "origin", Branch: "main", ExecutableTypes: []string{"task"}}}, Profiles: map[string]taskrunner.Profile{"live": {ID: "live", Input: runnerclient.RunInput{Workflow: runnerclient.Workflow{ID: "development", Version: 1}}}}, ResolveBase: func(context.Context, taskmodel.FixedWork, bool) (string, error) {
		baseCalls++
		return strings.Repeat("c", 40), nil
	}}
	prepareFixtureServiceEnvironment(t, s, svc)
	resolve := func(string, string) (string, error) { return "live", nil }
	controls := taskrunner.Controls{Tasks: svc, Store: s, ResolveProfile: resolve}
	now := time.Now().UTC()
	payload, _ := json.Marshal(api.ControlPayload{TaskID: tasks[0].TaskID, ExpectedTaskVersion: 1, ExpectedGraphRevision: 1, ExecutionProfileID: "live", ExecutionProfileDigest: strings.Repeat("a", 64)})
	req := api.ControlRequest{ProtocolVersion: 1, RequestID: strings.Repeat("e", 32), RepositoryIdentity: a.ProjectIdentity, InstanceID: "original", Action: "start_task", ExpectedControlRevision: 1, RequestedBy: "operator", CreatedAt: now, ExpiresAt: now.Add(time.Minute), Payload: payload}
	_, err := s.db.ExecContext(ctx, `CREATE TRIGGER lose_runner_control_receipt BEFORE UPDATE ON commands WHEN NEW.kind='runner_control' BEGIN SELECT RAISE(ABORT,'receipt fault'); END`)
	require(t, err)
	if receipt, err := controls.Handle(ctx, req, "original", 1); err == nil {
		t.Fatal("outer receipt loss was reported as known rejection", receipt)
	} else if receipt.Status != "applied" {
		t.Fatal("fixture did not reach committed reservation", receipt, err)
	}
	candidate, err := s.Candidate(ctx, a.ProjectIdentity, req.RequestID)
	require(t, err)
	if candidate.SubmitState != "pending" || candidate.RunID != "" || baseCalls != 1 {
		t.Fatal("start was not durably reserved before receipt loss")
	}
	_, err = s.db.ExecContext(ctx, `DROP TRIGGER lose_runner_control_receipt`)
	require(t, err)
	svc.Profiles = nil
	docs.body = nil
	controls.ResolveProfile = func(string, string) (string, error) {
		return "", errors.New("changed configuration must not be consulted for accepted replay")
	}
	replayed, err := controls.Handle(ctx, req, "replacement", 99)
	require(t, err)
	if replayed.Status != "applied" || replayed.RunID != candidate.ID || baseCalls != 1 {
		t.Fatal("accepted original start was lost or resubmitted", replayed)
	}
	changed := req
	changed.Action = "pause_dispatch"
	conflict, err := controls.Handle(ctx, changed, "replacement", 99)
	require(t, err)
	if conflict.Code != "idempotency_conflict" {
		t.Fatal("changed original control reused its receipt")
	}
	// An old unaccepted request does not become a fresh startup authorization.
	stale := req
	stale.RequestID = strings.Repeat("f", 32)
	denied, err := controls.Handle(ctx, stale, "replacement", 99)
	require(t, err)
	if denied.Status != "rejected" || denied.Code != "stale_instance" || baseCalls != 1 {
		t.Fatal("restart replay authorized a new start", denied)
	}
}
