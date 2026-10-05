package sqlite

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func (t *sqliteTx) PutProjectTaskAuthority(v executionmodel.ProjectTaskAuthority) error {
	if v.ProjectIdentity == "" || v.Authority != "native" || v.AuthorityVersion == 0 || v.DispatchMode != "manual" || !v.DispatchPaused || v.Repository == "" || v.Remote == "" || v.Branch == "" {
		return errors.New("native project authority requires identity, target, version and initial manual/paused dispatch")
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = now()
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = v.CreatedAt
	}
	result, err := t.tx.ExecContext(t.ctx, `INSERT INTO project_task_authorities(project_identity,authority,authority_version,graph_revision,repository,remote,branch,dispatch_mode,dispatch_paused,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(project_identity) DO UPDATE SET updated_at=excluded.updated_at WHERE project_task_authorities.authority=excluded.authority AND project_task_authorities.authority_version=excluded.authority_version AND project_task_authorities.graph_revision=excluded.graph_revision AND project_task_authorities.repository=excluded.repository AND project_task_authorities.remote=excluded.remote AND project_task_authorities.branch=excluded.branch AND project_task_authorities.dispatch_mode=excluded.dispatch_mode AND project_task_authorities.dispatch_paused=excluded.dispatch_paused`, v.ProjectIdentity, v.Authority, v.AuthorityVersion, v.GraphRevision, v.Repository, v.Remote, v.Branch, v.DispatchMode, boolInt(v.DispatchPaused), v.CreatedAt.UTC().Format(time.RFC3339Nano), v.UpdatedAt.UTC().Format(time.RFC3339Nano))
	return requireIdentityWrite(result, err, "authority_mismatch: native project authority is immutable without a compare-and-swap")
}

func (t *sqliteTx) UpdateProjectTaskAuthorityCAS(v executionmodel.ProjectTaskAuthority, expectedVersion uint64) error {
	if v.ProjectIdentity == "" || expectedVersion == 0 || v.AuthorityVersion != expectedVersion+1 || v.Authority != "native" || v.DispatchMode != "manual" && v.DispatchMode != "auto" {
		return nativeErr("authority_mismatch", "project authority update requires a valid next version and dispatch mode")
	}
	var old executionmodel.ProjectTaskAuthority
	var paused int
	var created, updated string
	err := t.tx.QueryRowContext(t.ctx, `SELECT authority,authority_version,graph_revision,repository,remote,branch,dispatch_mode,dispatch_paused,created_at,updated_at FROM project_task_authorities WHERE project_identity=?`, v.ProjectIdentity).Scan(&old.Authority, &old.AuthorityVersion, &old.GraphRevision, &old.Repository, &old.Remote, &old.Branch, &old.DispatchMode, &paused, &created, &updated)
	if err != nil {
		return requiredRecordError(err, "authority_mismatch", "native project authority was not found")
	}
	old.ProjectIdentity = v.ProjectIdentity
	old.DispatchPaused = paused != 0
	old.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	old.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	if old.AuthorityVersion != expectedVersion {
		return nativeErr("stale_version", fmt.Sprintf("expected authority version %d, found %d", expectedVersion, old.AuthorityVersion))
	}
	if v.GraphRevision != old.GraphRevision || v.Repository != old.Repository || v.Remote != old.Remote || v.Branch != old.Branch || !v.CreatedAt.Equal(old.CreatedAt) {
		return nativeErr("authority_mismatch", "project identity, target or graph revision cannot change through authority CAS")
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = now()
	}
	result, err := t.tx.ExecContext(t.ctx, `UPDATE project_task_authorities SET authority_version=?,dispatch_mode=?,dispatch_paused=?,updated_at=? WHERE project_identity=? AND authority_version=? AND graph_revision=?`, v.AuthorityVersion, v.DispatchMode, boolInt(v.DispatchPaused), v.UpdatedAt.UTC().Format(time.RFC3339Nano), v.ProjectIdentity, expectedVersion, v.GraphRevision)
	return requireIdentityWrite(result, err, "stale_version: project authority changed during compare-and-swap")
}

func (t *sqliteTx) AdvanceTaskGraphRevision(project string, expectedAuthorityVersion, expectedGraphRevision uint64) (uint64, error) {
	if project == "" || expectedAuthorityVersion == 0 {
		return 0, nativeErr("authority_mismatch", "graph revision update requires a project and authority version")
	}
	result, err := t.tx.ExecContext(t.ctx, `UPDATE project_task_authorities SET graph_revision=graph_revision+1,updated_at=? WHERE project_identity=? AND authority='native' AND authority_version=? AND graph_revision=?`, now().Format(time.RFC3339Nano), project, expectedAuthorityVersion, expectedGraphRevision)
	if err != nil {
		return 0, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if changed != 1 {
		return 0, nativeErr("stale_version", "project authority or graph revision changed")
	}
	return expectedGraphRevision + 1, nil
}

func (t *sqliteTx) PutTaskContract(project, taskID string, v taskmodel.TaskContract) error {
	if project == "" || taskID == "" || v.ContractID == "" || !taskmodel.ValidSHA256(v.Digest) {
		return errors.New("contract_mismatch: contract identity and digest are required")
	}
	if err := taskmodel.ValidateContract(v); err != nil {
		return err
	}
	var repository, remote, branch string
	if err := t.tx.QueryRowContext(t.ctx, `SELECT repository,remote,branch FROM project_task_authorities WHERE project_identity=? AND authority='native'`, project).Scan(&repository, &remote, &branch); err != nil {
		return requiredRecordError(err, "authority_mismatch", "task contract project has no native authority")
	}
	if !sameRepositoryTarget(v.Repository, repository) || v.Remote != remote || v.Branch != branch {
		return nativeErr("contract_mismatch", "task contract target differs from persisted project authority")
	}
	b, err := encodePayload(v)
	if err != nil {
		return err
	}
	res, err := t.tx.ExecContext(t.ctx, `INSERT INTO task_contracts(project_identity,contract_id,task_id,digest,payload) VALUES(?,?,?,?,?) ON CONFLICT(project_identity,contract_id) DO UPDATE SET digest=excluded.digest,payload=excluded.payload WHERE task_contracts.task_id=excluded.task_id AND task_contracts.digest=excluded.digest AND task_contracts.payload=excluded.payload`, project, v.ContractID, taskID, v.Digest, b)
	return requireIdentityWrite(res, err, "contract_mismatch: task contract is immutable or bound to another task")
}

func (t *sqliteTx) PutTaskBatch(v taskmodel.TaskBatchRecord, expectedAuthorityVersion, expectedGraphRevision uint64) error {
	if v.ProjectIdentity == "" || v.BatchID == "" || v.SchemaVersion != "task-batch/v1" || v.CommandID == "" || len(v.TaskIDs) == 0 || !taskmodel.ValidSHA256(v.PayloadDigest) {
		return errors.New("invalid task batch record")
	}
	if expectedAuthorityVersion == 0 {
		return nativeErr("authority_mismatch", "task batch requires an expected project authority version")
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = now()
	}
	var authority string
	if err := t.tx.QueryRowContext(t.ctx, `SELECT authority FROM project_task_authorities WHERE project_identity=?`, v.ProjectIdentity).Scan(&authority); err != nil {
		return requiredRecordError(err, "authority_mismatch", "task batch requires a registered native project")
	}
	if authority != "native" {
		return nativeErr("authority_mismatch", "task batch requires a registered native project")
	}
	seen := map[string]bool{}
	for localID, taskID := range v.TaskIDs {
		if strings.TrimSpace(localID) == "" || !taskmodel.CanonicalDecimalID(taskID) || seen[taskID] {
			return nativeErr("contract_mismatch", "task batch mapping must contain unique canonical task IDs and nonempty local keys")
		}
		seen[taskID] = true
	}
	b, err := encodePayload(v.TaskIDs)
	if err != nil {
		return err
	}
	var oldSchema, oldDigest, oldCommand string
	var oldMapping []byte
	err = t.tx.QueryRowContext(t.ctx, `SELECT schema_version,payload_digest,command_id,task_ids FROM task_batches WHERE project_identity=? AND batch_id=?`, v.ProjectIdentity, v.BatchID).Scan(&oldSchema, &oldDigest, &oldCommand, &oldMapping)
	if err == nil {
		if oldSchema != v.SchemaVersion || oldDigest != v.PayloadDigest || oldCommand != v.CommandID || !bytes.Equal(oldMapping, b) {
			return nativeErr("contract_mismatch", "task batch identity is already bound to different content")
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err = t.tx.ExecContext(t.ctx, `INSERT INTO task_batches(project_identity,batch_id,schema_version,payload_digest,created_at,command_id,task_ids) VALUES(?,?,?,?,?,?,?)`, v.ProjectIdentity, v.BatchID, v.SchemaVersion, v.PayloadDigest, v.CreatedAt.UTC().Format(time.RFC3339Nano), v.CommandID, b); err != nil {
		return err
	}
	_, err = t.AdvanceTaskGraphRevision(v.ProjectIdentity, expectedAuthorityVersion, expectedGraphRevision)
	return err
}

func (t *sqliteTx) PutTaskDependency(v taskmodel.TaskDependencyRecord) error {
	if v.ProjectIdentity == "" || v.PredecessorID == "" || v.SuccessorID == "" || v.PredecessorID == v.SuccessorID {
		return errors.New("invalid task dependency")
	}
	var authority string
	if err := t.tx.QueryRowContext(t.ctx, `SELECT authority FROM project_task_authorities WHERE project_identity=?`, v.ProjectIdentity).Scan(&authority); err != nil {
		return requiredRecordError(err, "authority_mismatch", "task dependency requires a registered native project")
	}
	if authority != "native" {
		return nativeErr("authority_mismatch", "task dependency requires a registered native project")
	}
	var successorStatus string
	if err := t.tx.QueryRowContext(t.ctx, `SELECT status FROM tasks WHERE project_identity=? AND task_id=?`, v.ProjectIdentity, v.SuccessorID).Scan(&successorStatus); err != nil {
		return requiredRecordError(err, "task_not_found", "dependency successor is not present")
	}
	if successorStatus != "open" {
		return nativeErr("immutable_task", "dependency successor must be open when an edge is inserted")
	}
	var delivered int
	if err := t.tx.QueryRowContext(t.ctx, `SELECT COUNT(*) FROM task_deliveries WHERE project_identity=? AND task_id=?`, v.ProjectIdentity, v.SuccessorID).Scan(&delivered); err != nil {
		return err
	}
	if delivered != 0 {
		return nativeErr("already_merged", "delivered task cannot acquire a predecessor")
	}
	_, err := t.tx.ExecContext(t.ctx, `INSERT INTO task_dependencies(project_identity,predecessor_id,successor_id) VALUES(?,?,?)`, v.ProjectIdentity, v.PredecessorID, v.SuccessorID)
	return err
}

func (t *sqliteTx) PutTaskDelivery(v executionmodel.TaskDeliveryRecord) error {
	if v.RecordedAt.IsZero() {
		v.RecordedAt = now()
	}
	var targetBranch string
	if err := t.tx.QueryRowContext(t.ctx, `SELECT branch FROM project_task_authorities WHERE project_identity=? AND authority='native'`, v.ProjectIdentity).Scan(&targetBranch); err != nil {
		return requiredRecordError(err, "authority_mismatch", "task delivery project has no native authority")
	}
	if err := validateNativeTaskDelivery(v, targetBranch); err != nil {
		return err
	}
	var oldWorkflow, oldInstance, oldRepository, oldURL, oldMerge, oldMerged string
	var oldNumber int64
	var oldEvidence []byte
	err := t.tx.QueryRowContext(t.ctx, `SELECT selected_workflow_run_id,pr_instance,pr_repository,pr_number,pr_url,merge_commit,merged_at,evidence FROM task_deliveries WHERE project_identity=? AND task_id=?`, v.ProjectIdentity, v.TaskID).Scan(&oldWorkflow, &oldInstance, &oldRepository, &oldNumber, &oldURL, &oldMerge, &oldMerged, &oldEvidence)
	if err == nil {
		if oldWorkflow == v.SelectedWorkflowRunID && oldInstance == v.PRInstance && oldRepository == v.PRRepository && oldNumber == v.PRNumber && oldURL == v.PRURL && oldMerge == v.MergeCommit && oldMerged == v.MergedAt.UTC().Format(time.RFC3339Nano) && bytes.Equal(oldEvidence, v.Evidence) {
			return nil
		}
		return nativeErr("contract_mismatch", "task already has a different authoritative delivery")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var taskStatus string
	if err := t.tx.QueryRowContext(t.ctx, `SELECT status FROM tasks WHERE project_identity=? AND task_id=?`, v.ProjectIdentity, v.TaskID).Scan(&taskStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nativeErr("task_not_found", "delivery requires an existing native task")
		}
		return err
	}
	if taskStatus == "discarded" {
		return nativeErr("task_discarded", "discarded task cannot receive a normal delivery")
	}
	if taskStatus != "in_progress" {
		return nativeErr("stale_version", "delivery requires the claimed task to be in progress")
	}
	var runPayload []byte
	if err := t.tx.QueryRowContext(t.ctx, `SELECT payload FROM workflow_runs WHERE project_identity=? AND workflow_run_id=? AND task_id=?`, v.ProjectIdentity, v.SelectedWorkflowRunID, v.TaskID).Scan(&runPayload); err != nil {
		return requiredRecordError(err, "contract_mismatch", "delivery must reference a workflow run belonging to the task")
	}
	var run executionmodel.WorkflowRunRecord
	if err := json.Unmarshal(runPayload, &run); err != nil {
		return err
	}
	var evidence executionmodel.TaskDeliveryEvidence
	if err := json.Unmarshal(v.Evidence, &evidence); err != nil {
		return err
	}
	if evidence.ExistingAcceptance != nil || run.WorkflowVersion != executionmodel.Version {
		return nativeErr("unsupported_schema", "delivery requires a worker-flow-v1 run")
	}
	if run.Abandonment != nil || run.SelectedResult == "" || run.PRURL != v.PRURL || run.Phase != "executing" && run.Phase != "waiting_decision" && run.Phase != "waiting_pr_merge" {
		return nativeErr("stale_version", "delivery requires the selected worker flow and its bound PR")
	}
	var executionPayload []byte
	if err := t.tx.QueryRowContext(t.ctx, "SELECT payload FROM executions WHERE project_identity=? AND workflow_run_id=? AND execution_id=?", v.ProjectIdentity, v.SelectedWorkflowRunID, run.SelectedResult).Scan(&executionPayload); err != nil {
		return requiredRecordError(err, "contract_mismatch", "delivery has no selected worker execution")
	}
	var execution executionmodel.ExecutionRecord
	if err := json.Unmarshal(executionPayload, &execution); err != nil {
		return err
	}
	sessionID, _ := executionSessionFacts(&execution)
	if sessionID == "" || execution.Workspace.Branch == "" {
		return nativeErr("contract_mismatch", "delivery has no bound worker session or branch")
	}
	_, err = t.tx.ExecContext(t.ctx, `INSERT INTO task_deliveries(project_identity,task_id,selected_workflow_run_id,pr_instance,pr_repository,pr_number,pr_url,merge_commit,merged_at,recorded_at,evidence) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, v.ProjectIdentity, v.TaskID, v.SelectedWorkflowRunID, v.PRInstance, v.PRRepository, v.PRNumber, v.PRURL, v.MergeCommit, v.MergedAt.UTC().Format(time.RFC3339Nano), v.RecordedAt.UTC().Format(time.RFC3339Nano), []byte(v.Evidence))
	return err
}

func validateNativeTaskDelivery(v executionmodel.TaskDeliveryRecord, targetBranch string) error {
	err := executionmodel.ValidateDelivery(v, targetBranch)
	var rejected *executionmodel.ResultError
	if errors.As(err, &rejected) {
		return nativeErr(rejected.Code, rejected.Message)
	}
	return err
}

func (t *sqliteTx) PutTask(v taskmodel.TaskRecord) error {
	if v.ProjectIdentity == "" || v.TaskID == "" || v.Status == "" {
		return errors.New("task identity and status are required")
	}
	if !taskmodel.ValidStatus(v.Status) {
		return fmt.Errorf("invalid task status %q", v.Status)
	}
	b, err := encodePayload(v)
	if err != nil {
		return err
	}
	var nativeAuthority string
	authorityErr := t.tx.QueryRowContext(t.ctx, `SELECT authority FROM project_task_authorities WHERE project_identity=?`, v.ProjectIdentity).Scan(&nativeAuthority)
	if authorityErr != nil && !errors.Is(authorityErr, sql.ErrNoRows) {
		return authorityErr
	}
	if errors.Is(authorityErr, sql.ErrNoRows) && v.Source == "native" {
		return nativeErr("authority_mismatch", "native task cannot be stored without a registered native project")
	}
	if authorityErr == nil {
		if nativeAuthority != "native" || v.Source != "native" || !taskmodel.CanonicalDecimalID(v.TaskID) || v.BatchID == "" || v.Title == "" || v.Type == "" || v.Priority < 0 || v.Priority > 4 || v.ContractID == "" || v.Version == 0 || v.CreatedAt.IsZero() || v.ExternalProvider != "" || v.ExternalProject != "" || v.ExternalID != "" || v.IdentityMapDigest != "" {
			return nativeErr("contract_mismatch", "native Task requires its original numeric ID, batch, type, priority and fixed contract")
		}
		var repository string
		if err = t.tx.QueryRowContext(t.ctx, `SELECT repository FROM project_task_authorities WHERE project_identity=? AND authority='native'`, v.ProjectIdentity).Scan(&repository); err != nil {
			return requiredRecordError(err, "authority_mismatch", "native task project binding is unavailable")
		}
		var reused int
		if err = t.tx.QueryRowContext(t.ctx, `SELECT COUNT(*) FROM tasks old JOIN project_task_authorities p ON p.project_identity=old.project_identity WHERE p.authority='native' AND p.repository=? AND old.task_id=? AND old.project_identity<>?`, repository, v.TaskID, v.ProjectIdentity).Scan(&reused); err != nil {
			return err
		}
		if reused > 0 {
			return nativeErr("immutable_task", "task ID is already used in the same repository identity")
		}
		if v.Status == "discarded" {
			if v.DiscardedAt == nil || strings.TrimSpace(v.DiscardReason) == "" || strings.TrimSpace(v.DiscardedBy) == "" || !executionbackend.StableExecutionID(v.DiscardRequestID) {
				return errors.New("discarded native task requires timestamp, reason, requester and request ID")
			}
		} else if v.DiscardedAt != nil || v.DiscardReason != "" || v.DiscardedBy != "" || v.DiscardRequestID != "" || len(v.CleanupPending) > 0 {
			return errors.New("discard audit fields are valid only for discarded native tasks")
		}
		if v.Status == "closed" {
			var deliveries int
			if err := t.tx.QueryRowContext(t.ctx, `SELECT COUNT(*) FROM task_deliveries WHERE project_identity=? AND task_id=?`, v.ProjectIdentity, v.TaskID).Scan(&deliveries); err != nil {
				return err
			}
			if deliveries != 1 {
				return errors.New("native task cannot close without its authoritative delivery evidence")
			}
		}
		var previous []byte
		previousErr := t.tx.QueryRowContext(t.ctx, `SELECT payload FROM tasks WHERE project_identity=? AND task_id=?`, v.ProjectIdentity, v.TaskID).Scan(&previous)
		if previousErr == nil {
			if bytes.Equal(previous, b) {
				return nil
			}
			return nativeErr("stale_version", "native task identity already exists; updates require an expected-version transaction")
		}
		if !errors.Is(previousErr, sql.ErrNoRows) {
			return previousErr
		}
	}
	result, err := t.tx.ExecContext(t.ctx, `INSERT INTO tasks(project_identity,task_id,external_provider,external_project,external_id,status,version,payload) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(project_identity,task_id) DO UPDATE SET status=excluded.status,version=excluded.version,payload=excluded.payload WHERE tasks.external_provider=excluded.external_provider AND tasks.external_project=excluded.external_project AND tasks.external_id=excluded.external_id`, v.ProjectIdentity, v.TaskID, v.ExternalProvider, v.ExternalProject, v.ExternalID, v.Status, v.Version, b)
	return requireIdentityWrite(result, err, "task ID belongs to another external identity")
}

func (t *sqliteTx) UpdateNativeTaskCAS(v taskmodel.TaskRecord, expectedVersion uint64) error {
	if v.ProjectIdentity == "" || !taskmodel.CanonicalDecimalID(v.TaskID) || expectedVersion == 0 {
		return nativeErr("contract_mismatch", "native task update requires project, canonical task ID and expected version")
	}
	var payload []byte
	var authority string
	if err := t.tx.QueryRowContext(t.ctx, `SELECT authority FROM project_task_authorities WHERE project_identity=?`, v.ProjectIdentity).Scan(&authority); err != nil {
		return requiredRecordError(err, "authority_mismatch", "task update requires a registered native project")
	}
	if authority != "native" {
		return nativeErr("authority_mismatch", "task update requires a registered native project")
	}
	if err := t.tx.QueryRowContext(t.ctx, `SELECT payload FROM tasks WHERE project_identity=? AND task_id=?`, v.ProjectIdentity, v.TaskID).Scan(&payload); err != nil {
		return err
	}
	var old taskmodel.TaskRecord
	if err := json.Unmarshal(payload, &old); err != nil {
		return err
	}
	if old.Source != "native" {
		return nativeErr("authority_mismatch", "task is not owned by the native authority")
	}
	if old.Version != expectedVersion {
		return nativeErr("stale_version", fmt.Sprintf("expected version %d, found %d", expectedVersion, old.Version))
	}
	if v.ProjectIdentity != old.ProjectIdentity || v.TaskID != old.TaskID || v.BatchID != old.BatchID || v.ContractID != old.ContractID || v.Type != old.Type || v.Priority != old.Priority || v.Title != old.Title || !slices.Equal(v.GoalIDs, old.GoalIDs) || v.IdentityMapDigest != old.IdentityMapDigest || v.Source != "native" || v.Version != expectedVersion+1 || !v.CreatedAt.Equal(old.CreatedAt) || v.ExternalProvider != "" || v.ExternalProject != "" || v.ExternalID != "" || v.IdentityMapDigest != "" {
		return nativeErr("contract_mismatch", "native task update cannot change its identity, scope, contract, or fixed fields")
	}
	allowed := map[string]map[string]bool{"open": {"in_progress": true, "discarded": true}, "in_progress": {"discarded": true, "closed": true}}
	if !allowed[old.Status][v.Status] {
		return nativeErr("stale_version", "native task status transition is not permitted")
	}
	if v.Status == "in_progress" {
		if old.Status != "open" || old.Claimant != "" || v.Claimant == "" {
			return nativeErr("stale_version", "claim requires an open task and a new claimant")
		}
	} else if v.Claimant != old.Claimant {
		return nativeErr("contract_mismatch", "claimant is preserved after claim")
	}
	if v.Status == "discarded" {
		if v.DiscardedAt == nil || strings.TrimSpace(v.DiscardReason) == "" || strings.TrimSpace(v.DiscardedBy) == "" || !executionbackend.StableExecutionID(v.DiscardRequestID) {
			return nativeErr("contract_mismatch", "discard requires timestamp, reason, requester and request ID")
		}
		var count int
		if err := t.tx.QueryRowContext(t.ctx, `SELECT COUNT(*) FROM task_deliveries WHERE project_identity=? AND task_id=?`, v.ProjectIdentity, v.TaskID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return nativeErr("stale_version", "task with a delivery cannot be discarded")
		}
	} else if v.DiscardedAt != nil || v.DiscardReason != "" || v.DiscardedBy != "" || v.DiscardRequestID != "" || len(v.CleanupPending) > 0 {
		return nativeErr("contract_mismatch", "discard audit fields are valid only for discarded tasks")
	}
	if v.Status == "closed" {
		if old.Claimant == "" || v.Claimant != old.Claimant {
			return nativeErr("stale_version", "unclaimed task cannot be closed")
		}
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = now()
	}
	b, err := encodePayload(v)
	if err != nil {
		return err
	}
	res, err := t.tx.ExecContext(t.ctx, `UPDATE tasks SET status=?,version=?,payload=? WHERE project_identity=? AND task_id=? AND version=?`, v.Status, v.Version, b, v.ProjectIdentity, v.TaskID, expectedVersion)
	if err != nil {
		return err
	}
	if err = requireIdentityWrite(res, err, "stale_version: native task changed during update"); err != nil {
		return err
	}
	if v.Status == "closed" {
		if err = t.completeNativeDeliveryWorkflow(v.ProjectIdentity, v.TaskID); err != nil {
			return err
		}
	}
	if v.Status == "discarded" || v.Status == "closed" {
		var authorityVersion, graphRevision uint64
		if err = t.tx.QueryRowContext(t.ctx, `SELECT authority_version,graph_revision FROM project_task_authorities WHERE project_identity=? AND authority='native'`, v.ProjectIdentity).Scan(&authorityVersion, &graphRevision); err != nil {
			return requiredRecordError(err, "authority_mismatch", "native task graph authority is unavailable")
		}
		_, err = t.AdvanceTaskGraphRevision(v.ProjectIdentity, authorityVersion, graphRevision)
		return err
	}
	return nil
}

func (t *sqliteTx) completeNativeDeliveryWorkflow(project, taskID string) error {
	var runID string
	var runPayload []byte
	var deliveryEvidence []byte
	err := t.tx.QueryRowContext(t.ctx, `SELECT d.selected_workflow_run_id,w.payload,d.evidence FROM task_deliveries d JOIN workflow_runs w ON w.project_identity=d.project_identity AND w.workflow_run_id=d.selected_workflow_run_id AND w.task_id=d.task_id WHERE d.project_identity=? AND d.task_id=?`, project, taskID).Scan(&runID, &runPayload, &deliveryEvidence)
	if err != nil {
		return requiredRecordError(err, "contract_mismatch", "task cannot close without its authoritative selected delivery")
	}
	var proof executionmodel.TaskDeliveryEvidence
	if err = json.Unmarshal(deliveryEvidence, &proof); err != nil {
		return nativeErr("inconsistent_delivery", "task delivery evidence is unreadable")
	}
	if proof.ExistingAcceptance != nil {
		return nativeErr("unsupported_schema", "old acceptance delivery is unsupported")
	}
	var run executionmodel.WorkflowRunRecord
	if err = json.Unmarshal(runPayload, &run); err != nil {
		return err
	}
	if run.WorkflowVersion == executionmodel.Version {
		var executionPayload []byte
		if run.SelectedResult == "" || run.PRURL != proof.URL || !proof.Merged || proof.State != "merged" {
			return nativeErr("stale_version", "selected worker flow no longer matches the merged PR")
		}
		if err = t.tx.QueryRowContext(t.ctx, `SELECT payload FROM executions WHERE project_identity=? AND workflow_run_id=? AND execution_id=?`, project, runID, run.SelectedResult).Scan(&executionPayload); err != nil {
			return requiredRecordError(err, "contract_mismatch", "selected worker flow has no execution")
		}
		var execution executionmodel.ExecutionRecord
		if err = json.Unmarshal(executionPayload, &execution); err != nil {
			return err
		}
		sessionID, _ := executionSessionFacts(&execution)
		if sessionID == "" || execution.Workspace.Branch == "" {
			return nativeErr("contract_mismatch", "selected worker flow lacks its session or branch")
		}
		delivery := executionmodel.TaskDeliveryRecord{TaskID: taskID, SelectedWorkflowRunID: runID, PRURL: proof.URL, MergeCommit: proof.MergeCommit, MergedAt: proof.MergedAt}
		state := executionmodel.State{
			WorkflowRunID: run.WorkflowRunID, TaskID: run.TaskID, WorkflowVersion: run.WorkflowVersion,
			Phase: run.Phase, PRURL: run.PRURL, PRState: run.PRState, WorkflowUpdatedAt: run.UpdatedAt,
		}
		next, completeErr := executionmodel.Complete(state, taskID, "closed", executionmodel.Delivery{
			TaskID: delivery.TaskID, WorkflowRunID: delivery.SelectedWorkflowRunID, PRURL: delivery.PRURL,
			MergeCommit: delivery.MergeCommit, MergedAt: delivery.MergedAt,
		}, now())
		if completeErr != nil {
			return completeErr
		}
		run.Phase, run.PRState, run.UpdatedAt = next.Phase, next.PRState, next.WorkflowUpdatedAt
		return t.PutWorkflowRun(run)
	}
	return nativeErr("unsupported_schema", "delivery completion requires worker-flow-v1")
}
