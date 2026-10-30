package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/taskrunner"
	"path/filepath"
	"strings"
	"testing"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func readinessRecord(project string) env.Record {
	return env.Record{ID: env.ID(), Project: project, Revision: 1, Kind: "adaptation", Profile: "one", Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"executable":"codex","model":"one"}`)}, Prepared: env.Prepared{Inputs: "basis"}, CreatedAt: time.Now().UTC(), State: "running", Requests: []env.Request{{ID: env.ID(), Original: json.RawMessage(`{}`)}}, Rounds: []env.Round{{Number: 1}}}
}
func invalidateReadiness(t *testing.T, s *SQLiteStore, project, key string, revision uint64) {
	t.Helper()
	ctx := context.Background()
	command := ports.CommandRecord{ProjectIdentity: project, CommandID: env.ID(), Kind: "project_environment", TargetID: project, IdempotencyKey: env.ID(), RequestedBy: "user", Payload: json.RawMessage(`{"action":"environment_invalidate"}`), Status: "pending", CreatedAt: time.Now().UTC()}
	_, _, err := s.EnqueueCommand(ctx, command)
	require(t, err)
	command.Status = "applied"
	command.Result = json.RawMessage(`{"status":"applied"}`)
	require(t, s.InvalidateEnvironment(ctx, project, key, "machine reset", revision, command))
}
func TestProjectEnvironmentReadinessCurrentAtomicityAndLateReady(t *testing.T) {
	ctx := context.Background()
	s := openDomainTestStore(t)
	record := readinessRecord("project")
	key, _ := env.Key(record.Execution)
	require(t, s.StartEnvironment(ctx, record, 0))
	current, err := s.CurrentEnvironment(ctx, "project", key)
	require(t, err)
	if current.ActivityID != record.ID || current.Revision != 1 {
		t.Fatal(current)
	}
	next := readinessRecord("project")
	if s.StartEnvironment(ctx, next, 1) == nil {
		t.Fatal("unsettled activity replaced")
	}
	if _, err := s.Environment(ctx, "project", next.ID); err == nil {
		t.Fatal("failed start persisted activity")
	}
	invalidateReadiness(t, s, "project", key, 1)
	record.State = "ready"
	record.Rounds[0].Settled = true
	record.Revision = 2
	require(t, s.SaveEnvironment(ctx, record, 1))
	revoked, err := s.CurrentEnvironment(ctx, "project", key)
	require(t, err)
	if _, err := env.Qualify(revoked, record, "basis"); err == nil {
		t.Fatal("late completion reactivated selection")
	}
	if s.StartEnvironment(ctx, next, 1) == nil {
		t.Fatal("stale revision selected activity")
	}
	if _, err := s.Environment(ctx, "project", next.ID); err == nil {
		t.Fatal("CAS failure left a new activity")
	}
	require(t, s.StartEnvironment(ctx, next, 2))
	selected, err := s.CurrentEnvironment(ctx, "project", key)
	require(t, err)
	if selected.ActivityID != next.ID || selected.Invalidated || selected.Revision != 3 {
		t.Fatal(selected)
	}
}
func TestProjectEnvironmentReadinessTransactionProofAndIdentity(t *testing.T) {
	ctx := context.Background()
	s := openDomainTestStore(t)
	record := readinessRecord("project")
	record.State = "ready"
	record.Rounds[0].Settled = true
	require(t, s.StartEnvironment(ctx, record, 0))
	key, _ := env.Key(record.Execution)
	current, err := s.CurrentEnvironment(ctx, "project", key)
	require(t, err)
	proof, err := env.Qualify(current, record, "basis")
	require(t, err)
	check := func(project string, p *env.Admission, execution runnercontract.ExecutionInput) error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		return verifyCandidateEnvironment(ctx, tx, project, p, execution)
	}
	require(t, check("project", &proof, record.Execution))
	if check("project", nil, record.Execution) == nil || check("other", &proof, record.Execution) == nil {
		t.Fatal("missing/foreign proof allowed")
	}
	changed := record.Execution
	changed.Settings = json.RawMessage(`{"executable":"other"}`)
	if check("project", &proof, changed) == nil {
		t.Fatal("proof accepted for different execution")
	}
	invalidateReadiness(t, s, "project", key, 1)
	if check("project", &proof, record.Execution) == nil {
		t.Fatal("revoked proof still allowed")
	}
	if !strings.Contains(check("project", &proof, record.Execution).Error(), "environment_not_ready") {
		t.Fatal("wrong denial")
	}
}

// Existing storage-owner scenarios construct a valid prerequisite explicitly;
// public system scenarios must instead obtain it through environment_start.
func reserveFixtureCandidate(t *testing.T, s *SQLiteStore, ctx context.Context, work taskmodel.FixedWork, c runnerbridge.Candidate, extra bool) error {
	t.Helper()
	var input runnercontract.RunInput
	if err := json.Unmarshal(c.Submit.Input, &input); err != nil {
		return err
	}
	if input.Execution.Adapter == "" {
		input.Execution = readinessRecord(c.Project).Execution
	}
	key, err := env.Key(input.Execution)
	if err != nil {
		return err
	}
	current, err := s.CurrentEnvironment(ctx, c.Project, key)
	if err != nil {
		if !errors.Is(err, env.ErrNotFound) {
			return err
		}
		record := readinessRecord(c.Project)
		record.Execution = input.Execution
		record.State = "ready"
		record.Rounds[0].Settled = true
		if err := s.StartEnvironment(ctx, record, 0); err != nil {
			return err
		}
		current, err = s.CurrentEnvironment(ctx, c.Project, key)
		if err != nil {
			return err
		}
	}
	record, err := s.Environment(ctx, c.Project, current.ActivityID)
	if err != nil {
		return err
	}
	proof, err := env.Qualify(current, record, record.Prepared.Inputs)
	if err != nil {
		return err
	}
	c.Environment = &proof
	c.Submit.Input, _ = json.Marshal(input)
	return s.ReserveRunnerCandidate(ctx, work, c, extra)
}

// The channels freeze the app observation before a competing public commit.
// No in-memory ready flag participates in the real reservation transaction.
func TestProjectEnvironmentReadinessReservationCommitOrder(t *testing.T) {
	for _, change := range []string{"invalidate", "start"} {
		for _, first := range []string{"change", "reserve"} {
			t.Run(change+"/"+first, func(t *testing.T) {
				ctx := context.Background()
				s, project, tasks := nativeAuditStore(t)
				require(t, s.transactSQLite(ctx, func(tx *sqliteTx) error {
					_, err := tx.tx.ExecContext(ctx, `UPDATE project_task_authorities SET dispatch_paused=0 WHERE project_identity=?`, project)
					return err
				}))
				graph, err := s.TaskGraphSnapshot(ctx, project)
				require(t, err)
				contract, err := s.TaskContract(ctx, project, tasks[0])
				require(t, err)
				work := taskmodel.FixedWork{Record: tasks[0], Contract: contract, Authority: graph.TaskGraph().Authority}
				record := readinessRecord(project)
				record.State = "ready"
				record.Rounds[0].Settled = true
				require(t, s.StartEnvironment(ctx, record, 0))
				key, _ := env.Key(record.Execution)
				current, err := s.CurrentEnvironment(ctx, project, key)
				require(t, err)
				proof, err := env.Qualify(current, record, record.Prepared.Inputs)
				require(t, err)
				input := runnerclient.RunInput{Execution: record.Execution, Title: work.Record.Title, Repository: runnerclient.Repository{URL: contract.Repository, Remote: contract.Remote, BaseBranch: contract.Branch, BaseRevision: strings.Repeat("a", 40)}, Documents: runnerclient.Documents{Spec: runnerclient.Document{Path: contract.SpecRef.Path}, Plan: runnerclient.Document{Path: contract.PlanRef.Path}}, Publication: runnerclient.Publication{BaseBranch: contract.Branch}}
				raw, _ := json.Marshal(input)
				candidate := runnerbridge.Candidate{Project: project, TaskID: tasks[0].TaskID, ID: "candidate", Revision: 1, StateDir: filepath.Join(t.TempDir(), "runner"), Claimant: "fixture", ContractDigest: contract.Digest, BaseRevision: strings.Repeat("a", 40), Participation: "active", Explicit: true, Environment: &proof, SubmitState: "pending", Submit: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "original-request", Input: raw}}
				release := make(chan struct{})
				done := make(chan error, 1)
				go func() { <-release; done <- s.ReserveRunnerCandidate(ctx, work, candidate, false) }()
				mutate := func() {
					if change == "invalidate" {
						invalidateReadiness(t, s, project, key, 1)
					} else {
						require(t, s.StartEnvironment(ctx, readinessRecord(project), 1))
					}
				}
				if first == "change" {
					mutate()
				}
				close(release)
				reserveErr := <-done
				if first == "reserve" {
					require(t, reserveErr)
					mutate()
				} else if reserveErr == nil || !strings.Contains(reserveErr.Error(), "environment_not_ready") {
					t.Fatal("stale proof reserved", reserveErr)
				}
				after, err := s.TaskGraphSnapshot(ctx, project)
				require(t, err)
				rows, err := s.RunnerCandidates(ctx, project, "")
				require(t, err)
				if first == "change" {
					if len(rows) != 0 || after.ByID[tasks[0].TaskID].Status != "open" || after.ByID[tasks[0].TaskID].Version != tasks[0].Version || after.Authority.GraphRevision != graph.Authority.GraphRevision {
						t.Fatal("rejected reservation mutated task or allocated candidate", rows)
					}
				} else {
					if len(rows) != 1 || rows[0].Number != 1 || after.ByID[tasks[0].TaskID].Status != "in_progress" {
						t.Fatal("original reservation lost")
					}
					require(t, s.ReserveRunnerCandidate(ctx, work, candidate, false)) // exact replay after change
					extra := candidate
					extra.ID = "extra"
					extra.Submit.RequestID = "extra-request"
					work.Record = after.ByID[tasks[0].TaskID]
					work.Authority = after.TaskGraph().Authority
					if err := s.ReserveRunnerCandidate(ctx, work, extra, true); err == nil || !strings.Contains(err.Error(), "environment_not_ready") {
						t.Fatal("changed qualification admitted extra", err)
					}
					rows, err = s.RunnerCandidates(ctx, project, "")
					require(t, err)
					if len(rows) != 1 {
						t.Fatal("denial allocated number")
					}
				}
			})
		}
	}
}

// These service-owner tests own the admission port; install an explicit valid
// storage prerequisite, while still using the production reservation gate.
func prepareFixtureServiceEnvironment(t *testing.T, s *SQLiteStore, service *taskrunner.Service) {
	t.Helper()
	ctx := context.Background()
	for id, profile := range service.Profiles {
		if profile.Input.Execution.Adapter == "" {
			profile.Input.Execution = readinessRecord(service.Project).Execution
			service.Profiles[id] = profile
		}
		key, err := env.Key(profile.Input.Execution)
		require(t, err)
		if _, err = s.CurrentEnvironment(ctx, service.Project, key); errors.Is(err, env.ErrNotFound) {
			record := readinessRecord(service.Project)
			record.Execution = profile.Input.Execution
			record.State = "ready"
			record.Rounds[0].Settled = true
			require(t, s.StartEnvironment(ctx, record, 0))
		} else {
			require(t, err)
		}
	}
	service.CheckExecution = func(ctx context.Context, id string, _ bool) error {
		key, err := env.Key(service.Profiles[id].Input.Execution)
		if err != nil {
			return err
		}
		current, err := s.CurrentEnvironment(ctx, service.Project, key)
		if err != nil {
			return err
		}
		record, err := s.Environment(ctx, service.Project, current.ActivityID)
		if err != nil {
			return err
		}
		_, err = env.Qualify(current, record, record.Prepared.Inputs)
		return err
	}
	service.ReserveExecution = func(ctx context.Context, work taskmodel.FixedWork, candidate runnerbridge.Candidate, extra bool) error {
		return reserveFixtureCandidate(t, s, ctx, work, candidate, extra)
	}
}
