package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"path/filepath"
)

func readRunnerCandidate(ctx context.Context, db schemaQuery, project, id string) (runnerstate.Candidate, error) {
	var body []byte
	err := db.QueryRowContext(ctx, `SELECT payload FROM runner_candidates WHERE project_identity=? AND candidate_id=?`, project, id).Scan(&body)
	if err != nil {
		return runnerstate.Candidate{}, err
	}
	var result runnerstate.Candidate
	if err := json.Unmarshal(body, &result); err != nil {
		return result, err
	}
	if result.Project != project || result.ID != id || result.Revision == 0 {
		return result, errors.New("candidate identity mismatch")
	}
	return result, nil
}
func (s *SQLiteStore) Candidate(ctx context.Context, project, id string) (runnerstate.Candidate, error) {
	return readRunnerCandidate(ctx, s.db, project, id)
}
func (s *SQLiteStore) RunnerCandidates(ctx context.Context, project, taskID string) ([]runnerstate.Candidate, error) {
	return readRunnerCandidates(ctx, s.db, project, taskID)
}

func readRunnerCandidates(ctx context.Context, db schemaQuery, project, taskID string) ([]runnerstate.Candidate, error) {
	query := `SELECT payload FROM runner_candidates WHERE project_identity=?`
	args := []any{project}
	if taskID != "" {
		query += ` AND task_id=?`
		args = append(args, taskID)
	}
	query += ` ORDER BY candidate_id`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []runnerstate.Candidate{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var candidate runnerstate.Candidate
		if err := json.Unmarshal(raw, &candidate); err != nil {
			return nil, err
		}
		result = append(result, candidate)
	}
	return result, rows.Err()
}
func (s *SQLiteStore) SaveCandidate(ctx context.Context, c runnerstate.Candidate, expected uint64, events []runnercontract.Event) error {
	return s.transactSQLite(ctx, func(tx *sqliteTx) error {
		old, err := readRunnerCandidate(ctx, tx.tx, c.Project, c.ID)
		if err != nil {
			return err
		}
		if old.Revision != expected || c.Revision != expected+1 {
			return nativeErr("stale_version", "candidate changed during CLI operation")
		}
		if old.TaskID != c.TaskID || old.StateDir != c.StateDir || old.Explicit != c.Explicit || old.Number != c.Number || old.Claimant != c.Claimant || old.ContractDigest != c.ContractDigest || old.BaseRevision != c.BaseRevision || digestValue(old.Submit) != digestValue(c.Submit) || old.RunID != "" && old.RunID != c.RunID || old.InputDigest != "" && old.InputDigest != c.InputDigest || old.Branch != "" && old.Branch != c.Branch || old.PRURL != "" && old.PRURL != c.PRURL || old.StopRequested && !c.StopRequested || old.SubmitObserveOnly && !c.SubmitObserveOnly || old.Participation != c.Participation {
			return nativeErr("immutable_task", "candidate update changed a frozen task association")
		}
		if old.StartDigest != c.StartDigest || old.ProfileID != c.ProfileID || old.ProfileName != c.ProfileName || old.ProfileDigest != c.ProfileDigest || old.RunnerBuild != c.RunnerBuild || !old.CreatedAt.Equal(c.CreatedAt) {
			return nativeErr("immutable_task", "candidate update changed its original start identity")
		}
		if c.RunID != "" && c.SubmitState != "accepted" {
			return nativeErr("contract_mismatch", "Run binding requires its accepted original submission")
		}
		body, err := json.Marshal(c)
		if err != nil {
			return err
		}
		result, err := tx.tx.ExecContext(ctx, `UPDATE runner_candidates SET revision=?,run_id=?,payload=? WHERE project_identity=? AND candidate_id=? AND revision=?`, c.Revision, c.RunID, body, c.Project, c.ID, expected)
		if err := requireIdentityWrite(result, err, "stale_version: candidate changed"); err != nil {
			return err
		}
		for _, event := range events {
			if event.RunID != c.RunID || event.EventID == "" {
				return errors.New("event belongs to another candidate Run")
			}
			payload, _ := json.Marshal(event)
			var previous []byte
			err := tx.tx.QueryRowContext(ctx, `SELECT payload FROM runner_events WHERE project_identity=? AND candidate_id=? AND event_id=?`, c.Project, c.ID, event.EventID).Scan(&previous)
			if err == nil {
				if string(previous) != string(payload) {
					return errors.New("conflicting duplicate Runner event")
				}
				continue
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if _, err := tx.tx.ExecContext(ctx, `INSERT INTO runner_events(project_identity,candidate_id,event_id,payload) VALUES(?,?,?,?)`, c.Project, c.ID, event.EventID, payload); err != nil {
				return err
			}
		}
		return nil
	})
}

// ReserveRunnerCandidate atomically claims task work and fixes the original
// CLI submission. It performs no external execution or platform operation.
func (s *SQLiteStore) ReserveRunnerCandidate(ctx context.Context, work taskmodel.FixedWork, c runnerstate.Candidate, candidate bool) error {
	if c.Project != work.Record.ProjectIdentity || c.TaskID != work.Record.TaskID || c.ID == "" || c.Revision != 1 || c.SubmitState != "pending" || c.RunID != "" || c.Submit.Protocol != runnercontract.Protocol || c.Submit.RequestID == "" || !json.Valid(c.Submit.Input) || !filepath.IsAbs(c.StateDir) || c.Claimant == "" || c.ContractDigest != work.Contract.Digest || !taskmodel.ValidCommit(c.BaseRevision) || c.Participation != "active" {
		return nativeErr("contract_mismatch", "candidate reservation requires complete fixed task and Runner inputs")
	}
	var input runnercontract.RunInput
	if json.Unmarshal(c.Submit.Input, &input) != nil || input.Title != work.Record.Title || input.Repository.URL != work.Contract.Repository || input.Repository.Remote != work.Contract.Remote || input.Repository.BaseBranch != work.Contract.Branch || input.Repository.BaseRevision != c.BaseRevision || input.Publication.BaseBranch != work.Contract.Branch {
		return nativeErr("contract_mismatch", "Runner input differs from the frozen task repository")
	}
	matches := func(doc runnercontract.Document, ref taskmodel.FileReference) bool {
		return doc.Kind == "git" && doc.Path == ref.Path && doc.Commit == ref.Commit && doc.Content == nil
	}
	if !matches(input.Documents.Spec, work.Contract.SpecRef) || !matches(input.Documents.Plan, work.Contract.PlanRef) {
		return nativeErr("contract_mismatch", "Runner Spec/Plan differ from the task contract")
	}
	refs := append(append([]taskmodel.FileReference{}, work.Contract.CommonRefs...), work.Contract.AcceptanceRefs...)
	want := map[string]taskmodel.FileReference{}
	for _, ref := range refs {
		if ref.Path != work.Contract.SpecRef.Path && ref.Path != work.Contract.PlanRef.Path {
			want[ref.Path] = ref
		}
	}
	for _, doc := range input.Documents.References {
		ref, ok := want[doc.Path]
		if !ok || !matches(doc, ref) {
			return nativeErr("contract_mismatch", "Runner reference differs from the task contract")
		}
		delete(want, doc.Path)
	}
	if len(want) != 0 {
		return nativeErr("contract_mismatch", "Runner input omits fixed task references")
	}
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		previous, lookupErr := readRunnerCandidate(ctx, tx.tx, c.Project, c.ID)
		if lookupErr == nil {
			if previous.TaskID != c.TaskID || digestValue(previous.Submit) != digestValue(c.Submit) || previous.BaseRevision != c.BaseRevision || previous.ContractDigest != c.ContractDigest || previous.Claimant != c.Claimant || previous.Explicit != c.Explicit || previous.StateDir != c.StateDir || previous.StartDigest != c.StartDigest || previous.ProfileID != c.ProfileID || previous.ProfileName != c.ProfileName || previous.ProfileDigest != c.ProfileDigest || previous.RunnerBuild != c.RunnerBuild || !previous.CreatedAt.Equal(c.CreatedAt) {
				return nativeErr("idempotency_conflict", "candidate ID belongs to another original request")
			}
			return errAtomicReplay
		}
		if !errors.Is(lookupErr, sql.ErrNoRows) {
			return lookupErr
		}
		graph, err := tx.TaskGraphSnapshot(ctx, c.Project)
		if err != nil {
			return err
		}
		if graph.Authority.AuthorityVersion != work.Authority.AuthorityVersion || graph.Authority.GraphRevision != work.Authority.GraphRevision {
			return nativeErr("stale_version", "task graph changed before reservation")
		}
		if graph.Authority.DispatchPaused {
			return nativeErr("dispatch_paused", "dispatch is paused")
		}
		if !candidate && (c.Explicit && graph.Authority.DispatchMode != "manual" || !c.Explicit && graph.Authority.DispatchMode != "auto") {
			return nativeErr("wrong_mode", "dispatch mode differs from the original start")
		}
		if candidate && !c.Explicit {
			return nativeErr("wrong_mode", "new candidates require an explicit start")
		}
		var active, number, conflictingBase int
		if err := tx.tx.QueryRowContext(ctx, `SELECT coalesce(sum(CASE WHEN json_extract(payload,'$.participation')='active' THEN 1 ELSE 0 END),0),coalesce(max(json_extract(payload,'$.number')),0),coalesce(sum(CASE WHEN json_extract(payload,'$.base_revision')<>? THEN 1 ELSE 0 END),0) FROM runner_candidates WHERE project_identity=? AND task_id=?`, c.BaseRevision, c.Project, c.TaskID).Scan(&active, &number, &conflictingBase); err != nil {
			return err
		}
		if active >= 5 {
			return nativeErr("candidate_limit_reached", "five candidates already participate")
		}
		if candidate && conflictingBase > 0 {
			return nativeErr("contract_mismatch", "candidate common base changed")
		}
		c.Number = number + 1
		if !c.Explicit {
			var occupied int
			if err := tx.tx.QueryRowContext(ctx, `SELECT count(*) FROM runner_candidates WHERE project_identity=? AND json_extract(payload,'$.participation')='active' AND json_extract(payload,'$.submit_state') NOT IN ('rejected','cancelled') AND coalesce(json_extract(payload,'$.projection.status'),'queued') NOT IN ('pr_ready','stopped')`, c.Project).Scan(&occupied); err != nil {
				return err
			}
			if graph.ExecutionSlotBusy || occupied > 0 {
				return nativeErr("slot_busy", "an original candidate is active or uncertain")
			}
		}
		current, ok := graph.ByID[c.TaskID]
		if !ok || current.Version != work.Record.Version || !sameTaskRecord(current, work.Record) {
			return nativeErr("stale_version", "task changed before reservation")
		}
		eligibility := current
		if candidate {
			eligibility.Status = "open"
		}
		if ready, blockers := taskmodel.DependencyStateForTask(eligibility, graph.TaskGraph()); !ready {
			return taskmodel.DependencyClaimError(blockers)
		}
		if candidate {
			if current.Status != "in_progress" || current.Claimant != c.Claimant {
				return nativeErr("stale_version", "candidate requires original claimed task")
			}
		} else {
			next, err := taskmodel.Claim(current, c.Claimant, now())
			if err != nil {
				return err
			}
			if err := tx.UpdateNativeTaskCAS(next, current.Version); err != nil {
				return err
			}
		}
		contract, err := tx.TaskContract(ctx, c.Project, current)
		if err != nil {
			return err
		}
		if contract.Digest != c.ContractDigest {
			return nativeErr("immutable_task", "fixed contract changed")
		}
		body, err := json.Marshal(c)
		if err != nil {
			return err
		}
		_, err = tx.tx.ExecContext(ctx, `INSERT INTO runner_candidates(project_identity,candidate_id,task_id,revision,request_id,run_id,payload) VALUES(?,?,?,?,?,'',?)`, c.Project, c.ID, c.TaskID, c.Revision, c.Submit.RequestID, body)
		return err
	})
	if errors.Is(err, errAtomicReplay) {
		return nil
	}
	return err
}

// requestRunnerTaskStops runs inside the task's terminal transaction. A late
// CLI acceptance keeps this obligation because SaveCandidate cannot clear it.
func (tx *sqliteTx) requestRunnerTaskStops(project, taskID string) error {
	rows, err := tx.tx.QueryContext(tx.ctx, `SELECT payload FROM runner_candidates WHERE project_identity=? AND task_id=?`, project, taskID)
	if err != nil {
		return err
	}
	values := []runnerstate.Candidate{}
	for rows.Next() {
		var body []byte
		if err := rows.Scan(&body); err != nil {
			rows.Close()
			return err
		}
		var c runnerstate.Candidate
		if err := json.Unmarshal(body, &c); err != nil {
			rows.Close()
			return err
		}
		values = append(values, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, c := range values {
		if c.StopRequested {
			continue
		}
		c.StopRequested = true
		c.InputsRevoked = false
		c.Revision++
		if c.Participation == "active" {
			c.Participation = "abandoned"
		}
		for i := range c.Actions {
			if c.Actions[i].State == "pending" {
				c.Actions[i].State = "cancelled"
			}
		}
		body, err := json.Marshal(c)
		if err != nil {
			return err
		}
		result, err := tx.tx.ExecContext(tx.ctx, `UPDATE runner_candidates SET revision=?,payload=? WHERE project_identity=? AND candidate_id=? AND revision=?`, c.Revision, body, project, c.ID, c.Revision-1)
		if err := requireIdentityWrite(result, err, "stale_version: terminal candidate changed"); err != nil {
			return err
		}
	}
	return nil
}
