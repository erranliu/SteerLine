package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	taskapp "github.com/erranliu/SteerLine/internal/task"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

type runnerTaskDocuments struct {
	repository string
	body       []byte
}

func (d *runnerTaskDocuments) RepositoryTarget(context.Context) (string, error) {
	return d.repository, nil
}
func (d *runnerTaskDocuments) ReadCommittedRegularFile(context.Context, taskmodel.FileReference) ([]byte, error) {
	return d.body, nil
}

func TestRunnerPresetFreeze(t *testing.T) {
	ctx := context.Background()
	s := openDomainTestStore(t)
	a := taskmodel.ProjectTaskAuthority{ProjectIdentity: "runner-service", Authority: "native", AuthorityVersion: 1, Repository: "https://example.invalid/Org/Repo.git", Remote: "origin", Branch: "main", DispatchMode: "manual", DispatchPaused: true}
	require(t, s.transactSQLite(ctx, func(tx *sqliteTx) error { return tx.PutProjectTaskAuthority(a) }))
	docs := &runnerTaskDocuments{repository: a.Repository, body: []byte("# Acceptance\nfixed spec and plan\n")}
	sum := sha256.Sum256(docs.body)
	ref := taskmodel.FileReference{Path: "docs/task.spec.md", Commit: strings.Repeat("a", 40), SHA256: hex.EncodeToString(sum[:])}
	tasks := nativeAuditRecordsWithReference(t, s, a, ref)
	require(t, s.transactSQLite(ctx, func(tx *sqliteTx) error {
		_, err := tx.tx.ExecContext(ctx, `UPDATE project_task_authorities SET dispatch_paused=0 WHERE project_identity=?`, a.ProjectIdentity)
		return err
	}))
	baseCalls := 0
	svc := taskrunner.Service{Project: a.ProjectIdentity, Claimant: "runner-test", StateDir: filepath.Join(t.TempDir(), "runner"), DefaultProfile: "live", Store: s,
		Tasks:    taskapp.Service{Reader: s, Documents: docs, Input: taskapp.Input{Project: a.ProjectIdentity, Authority: "native", Remote: "origin", Branch: "main", ExecutableTypes: []string{"task"}}},
		Profiles: map[string]taskrunner.Profile{"live": {ID: "live", Name: "Original frozen preset", Input: runnerclient.RunInput{Workflow: runnerclient.Workflow{ID: "development", Version: 1}, Execution: runnerclient.ExecutionInput{Adapter: "codex-app-server", Target: runnerclient.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"executable":"fixed-native"}`)}, Publication: runnerclient.Publication{Provider: "command", Repository: "Org/Repo", Settings: json.RawMessage(`{"base_url":"https://example.invalid"}`)}}}},
		ResolveBase: func(context.Context, taskmodel.FixedWork, bool) (string, error) {
			baseCalls++
			return strings.Repeat("c", 40), nil
		},
	}
	graph, err := s.TaskGraphSnapshot(ctx, a.ProjectIdentity)
	require(t, err)
	start := taskrunner.Start{GraphRevision: graph.Authority.GraphRevision, RequestID: strings.Repeat("d", 32), TaskID: tasks[0].TaskID, TaskVersion: 1, Explicit: true}
	stale := start
	stale.GraphRevision++
	if _, err := svc.Start(ctx, stale); err == nil || baseCalls != 0 {
		t.Fatal("stale displayed graph acquired input or base", err, baseCalls)
	}

	first, err := svc.Start(ctx, start)
	require(t, err)
	var fixed runnerclient.RunInput
	require(t, json.Unmarshal(first.Submit.Input, &fixed))
	if fixed.Repository.BaseRevision != strings.Repeat("c", 40) || fixed.Documents.Spec.Commit != ref.Commit || fixed.Correlation["steerline_task_id"] != start.TaskID || first.ProfileID != "live" || first.Number != 1 {
		t.Fatal("start failed to freeze its exact task/profile/base", first, fixed)
	}
	if first.ProfileDigest != runnercontract.ConfigurationDigest(fixed.Execution, fixed.Workflow) || first.ProfileName != svc.Profiles["live"].Name || first.RunnerBuild != svc.RunnerBuild {
		t.Fatal("candidate did not atomically freeze preset and build", first)
	}
	for _, change := range []func(*runnercontract.BuildPin){func(p *runnercontract.BuildPin) { p.SHA256 = "other" }, func(p *runnercontract.BuildPin) { p.ResourceDigest = "other" }} {
		changed := first
		changed.Revision++
		change(&changed.RunnerBuild)
		if err := s.SaveCandidate(ctx, changed, first.Revision, nil); err == nil {
			t.Fatal("frozen build overwritten")
		}
	}
	changed := first
	changed.Revision++
	changed.ProfileName = "renamed"
	if err := s.SaveCandidate(ctx, changed, first.Revision, nil); err == nil {
		t.Fatal("frozen name overwritten")
	}
	// The receipt must survive mutable defaults, missing source material and an
	// advanced task version without touching the external environment again.
	docs.body = []byte("changed after acceptance")
	svc.Profiles = nil
	again, err := svc.Start(ctx, start)
	require(t, err)
	if digestValue(first) != digestValue(again) || baseCalls != 1 {
		t.Fatal("original start was rebuilt instead of replayed")
	}
	start.ProfileID = "different"
	if _, err := svc.Start(ctx, start); err == nil {
		t.Fatal("changed start body reused the original request ID")
	}
	withdrawn, err := svc.Stop(ctx, first.ID, first.Revision)
	require(t, err)
	if !withdrawn.StopRequested || withdrawn.Participation != "abandoned" || withdrawn.InputsRevoked || withdrawn.RunID != "" {
		t.Fatal("withdrawal invented native stop evidence", withdrawn)
	}
	current, err := s.TaskGraphSnapshot(ctx, a.ProjectIdentity)
	require(t, err)
	if current.ByID[start.TaskID].Status != "in_progress" {
		t.Fatal("withdrawing a candidate incorrectly closed its task")
	}
}
