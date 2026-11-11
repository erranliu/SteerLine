package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestReadinessAdmissionPolicyReservation(t *testing.T) {
	for _, mode := range []string{"manual", "auto"} {
		for _, skip := range []bool{false, true} {
			t.Run(mode+"/skip="+map[bool]string{false: "false", true: "true"}[skip], func(t *testing.T) {
				ctx := context.Background()
				s, project, tasks := nativeAuditStore(t)
				path := s.path
				require(t, s.Close())
				s, err := OpenSQLiteStoreWithAdmissionPolicy(path, env.NewAdmissionPolicy(!skip))
				require(t, err)
				defer s.Close()
				require(t, s.transactSQLite(ctx, func(tx *sqliteTx) error {
					_, err := tx.tx.ExecContext(ctx, `UPDATE project_task_authorities SET dispatch_mode=? WHERE project_identity=?`, mode, project)
					return err
				}))
				graph, err := s.TaskGraphSnapshot(ctx, project)
				require(t, err)
				contract, err := s.TaskContract(ctx, project, tasks[0])
				require(t, err)
				work := taskmodel.FixedWork{Record: tasks[0], Contract: contract, Authority: graph.TaskGraph().Authority}
				input := runnerclient.RunInput{Title: work.Record.Title, Execution: readinessRecord(project).Execution,
					Repository:  runnerclient.Repository{URL: contract.Repository, Remote: contract.Remote, BaseBranch: contract.Branch, BaseRevision: strings.Repeat("a", 40)},
					Documents:   runnerclient.Documents{Spec: runnerclient.Document{Path: contract.SpecRef.Path}, Plan: runnerclient.Document{Path: contract.PlanRef.Path}},
					Publication: runnerclient.Publication{BaseBranch: contract.Branch}}
				raw, _ := json.Marshal(input)
				candidate := runnerstate.Candidate{Project: project, TaskID: tasks[0].TaskID, ID: "developer-candidate", Revision: 1,
					StateDir: filepath.Join(t.TempDir(), "runner"), Claimant: "fixture", ContractDigest: contract.Digest, BaseRevision: input.Repository.BaseRevision,
					Participation: "active", Explicit: mode == "manual", EnvironmentReadinessSkipped: !skip,
					SubmitState: "pending", Submit: runnerclient.Request{Protocol: runnercontract.Protocol, RequestID: "developer-request", Input: raw}}
				original := candidate
				err = s.ReserveRunnerCandidate(ctx, work, candidate, false)
				if !skip {
					if err == nil || !strings.Contains(err.Error(), "environment_not_ready") {
						t.Fatal("payload authorized a bypass without writer policy", err)
					}
					rows, err := s.RunnerCandidates(ctx, project, "")
					require(t, err)
					if len(rows) != 0 {
						t.Fatal("denial created a candidate")
					}
					return
				}
				require(t, err)
				saved, err := s.Candidate(ctx, project, candidate.ID)
				require(t, err)
				if !saved.EnvironmentReadinessSkipped || saved.Environment != nil {
					t.Fatal("missing skip evidence", saved)
				}
				require(t, s.ReserveRunnerCandidate(ctx, work, candidate, false))
				// An additional candidate also uses the writer policy, while normal
				// task version and dispatch checks remain authoritative.
				graph, err = s.TaskGraphSnapshot(ctx, project)
				require(t, err)
				work.Record, work.Authority = graph.ByID[candidate.TaskID], graph.TaskGraph().Authority
				candidate.ID, candidate.Submit.RequestID, candidate.Explicit = "developer-extra", "extra-request", true
				require(t, s.ReserveRunnerCandidate(ctx, work, candidate, true))
				candidate.ID, candidate.Submit.RequestID = "developer-stale", "stale-request"
				work.Record.Version++
				if err := s.ReserveRunnerCandidate(ctx, work, candidate, true); err == nil || !strings.Contains(err.Error(), "stale_version") {
					t.Fatal("skip bypassed task version", err)
				}
				// Restarting with defaults restores the gate for new reservations.
				require(t, s.Close())
				strict, err := OpenSQLiteStore(path)
				require(t, err)
				defer strict.Close()
				require(t, strict.ReserveRunnerCandidate(ctx, work, original, false))
				retained, err := strict.Candidate(ctx, project, original.ID)
				require(t, err)
				if !retained.EnvironmentReadinessSkipped {
					t.Fatal("replay changed historical admission evidence")
				}
				work.Record.Version--
				if err := strict.ReserveRunnerCandidate(ctx, work, candidate, true); err == nil || !strings.Contains(err.Error(), "environment_not_ready") {
					t.Fatal("restart retained bypass", err)
				}
			})
		}
	}
}
