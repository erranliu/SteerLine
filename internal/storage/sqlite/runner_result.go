package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"strings"
	"time"
)

type RunnerResult = storageports.RunnerResult

// CommitRunnerTaskResult consumes a fresh public Runner PR fact. Task closure,
// unique delivery, graph release and every candidate stop obligation commit
// together. No Runner execution state is written by this transaction.
func (s *SQLiteStore) CommitRunnerTaskResult(ctx context.Context, input RunnerResult) (executionmodel.TaskDeliveryReceipt, error) {
	var receipt executionmodel.TaskDeliveryReceipt
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		graph, err := tx.TaskGraphSnapshot(ctx, input.Project)
		if err != nil {
			return err
		}
		record, ok := graph.ByID[input.TaskID]
		if !ok {
			return nativeErr("task_not_found", "task was not found")
		}
		if delivery, ok := graph.Deliveries[input.TaskID]; ok {
			if record.Status != "closed" || delivery.SelectedWorkflowRunID != input.CandidateID {
				return nativeErr("inconsistent_delivery", "task already has another authoritative delivery")
			}
			saved, found, err := tx.taskResult(ctx, input.Project, input.CandidateID)
			if err != nil {
				return err
			}
			if !found || saved.Input.ExpectedTaskVersion != input.TaskVersion {
				return nativeErr("stale_version", "delivery retry changed the original task version")
			}
			receipt = executionmodel.TaskDeliveryReceipt{TaskID: input.TaskID, WorkflowRunID: input.CandidateID, TaskVersion: saved.TaskVersion, GraphRevision: saved.GraphRevision, Status: "closed", Delivery: delivery}
			return errAtomicReplay
		}
		if record.Status == "discarded" {
			return nativeErr("task_discarded", "discarded task cannot receive delivery")
		}
		if record.Status != "in_progress" || record.Version != input.TaskVersion || graph.Authority.AuthorityVersion != input.AuthorityVersion || graph.Authority.GraphRevision != input.GraphRevision {
			return nativeErr("stale_version", "task or graph changed during PR verification")
		}
		c, err := readRunnerCandidate(ctx, tx.tx, input.Project, input.CandidateID)
		if err != nil {
			return err
		}
		if c.TaskID != input.TaskID || c.Revision != input.CandidateRevision || c.Participation != "active" || c.StopRequested || c.RunID == "" || c.SubmitState != "accepted" || c.Claimant != record.Claimant {
			return nativeErr("candidate_inactive", "selected Run binding changed during verification")
		}
		contract, err := tx.TaskContract(ctx, input.Project, record)
		if err != nil {
			return err
		}
		if c.ContractDigest != contract.Digest {
			return nativeErr("contract_mismatch", "candidate lost the fixed task contract")
		}
		provider, proof, err := runnerDeliveryProof(c, input.VerificationStarted, graph.Authority.Branch)
		if err != nil {
			return err
		}
		var reused int
		if err := tx.tx.QueryRowContext(ctx, `SELECT count(*) FROM runner_candidates WHERE project_identity=? AND candidate_id<>? AND json_extract(payload,'$.pr_url')=?`, input.Project, c.ID, proof.URL).Scan(&reused); err != nil {
			return err
		}
		if reused > 0 {
			return nativeErr("inconsistent_delivery", "PR is bound to another task candidate")
		}
		at := now()
		body, _ := json.Marshal(proof)
		delivery := executionmodel.TaskDeliveryRecord{ProjectIdentity: input.Project, TaskID: input.TaskID, SelectedWorkflowRunID: c.ID, PRInstance: strings.TrimSuffix(provider.BaseURL, "/"), PRRepository: proof.Repository, PRNumber: proof.PRNumber, PRURL: proof.URL, MergeCommit: proof.MergeCommit, MergedAt: proof.MergedAt, RecordedAt: at, Evidence: body}
		if err := validateNativeTaskDelivery(delivery, graph.Authority.Branch); err != nil {
			return err
		}
		declaration, err := taskmodel.DeclareResult(record, contract, taskmodel.Result{ProjectIdentity: input.Project, TaskID: input.TaskID, ContractID: record.ContractID, ContractDigest: contract.Digest, Claimant: record.Claimant, ExpectedTaskVersion: input.TaskVersion, ExpectedGraphRevision: input.GraphRevision, ExpectedSetVersion: digestValue(struct{ ID, Run, Digest, Branch, PR string }{c.ID, c.RunID, c.InputDigest, c.Branch, c.PRURL}), ResultID: c.ID, RequestID: "delivery:" + c.ID, Conclusion: "delivered"}, at)
		if err != nil {
			return err
		}
		if _, err := tx.tx.ExecContext(ctx, `INSERT INTO task_deliveries(project_identity,task_id,selected_workflow_run_id,pr_instance,pr_repository,pr_number,pr_url,merge_commit,merged_at,recorded_at,evidence) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, input.Project, input.TaskID, c.ID, delivery.PRInstance, delivery.PRRepository, delivery.PRNumber, delivery.PRURL, delivery.MergeCommit, delivery.MergedAt.UTC().Format(time.RFC3339Nano), at.Format(time.RFC3339Nano), body); err != nil {
			return err
		}
		if err := tx.UpdateNativeTaskCAS(declaration.After, record.Version); err != nil {
			return err
		}
		result := taskmodel.ResultReceipt{Input: declaration.Input, TaskVersion: declaration.After.Version, GraphRevision: input.GraphRevision + 1, Status: "closed"}
		resultBody, _ := json.Marshal(result)
		resultInput, _ := json.Marshal(declaration.Input)
		if _, err := tx.tx.ExecContext(ctx, `INSERT INTO task_results(project_identity,task_id,result_id,request_id,input,receipt) VALUES(?,?,?,?,?,?)`, input.Project, input.TaskID, c.ID, declaration.Input.RequestID, resultInput, resultBody); err != nil {
			return err
		}
		receipt = executionmodel.TaskDeliveryReceipt{TaskID: input.TaskID, WorkflowRunID: c.ID, TaskVersion: result.TaskVersion, GraphRevision: result.GraphRevision, Status: "closed", Delivery: delivery}
		event, _ := json.Marshal(receipt)
		_, err = tx.AppendEvent(storageports.DomainEvent{ProjectIdentity: input.Project, TaskID: input.TaskID, WorkflowRunID: c.ID, ExternalSource: "runner-task-delivery", ExternalID: proof.URL + ":" + proof.MergeCommit, Type: "task.delivered", ReceivedAt: at, Payload: event})
		return err
	})
	if errors.Is(err, errAtomicReplay) {
		err = nil
	}
	return receipt, err
}

func runnerDeliveryProof(c runnerbridge.Candidate, started time.Time, branch string) (executionmodel.PRProviderConfig, executionmodel.TaskDeliveryEvidence, error) {
	var input runnerclient.RunInput
	var settings struct {
		BaseURL string `json:"base_url"`
	}
	if json.Unmarshal(c.Submit.Input, &input) != nil || json.Unmarshal(input.Publication.Settings, &settings) != nil {
		return executionmodel.PRProviderConfig{}, executionmodel.TaskDeliveryEvidence{}, nativeErr("contract_mismatch", "frozen publication configuration is invalid")
	}
	provider := executionmodel.PRProviderConfig{Kind: input.Publication.Provider, BaseURL: settings.BaseURL, Repository: input.Publication.Repository}
	if c.Projection == nil || c.Projection.RunID != c.RunID || c.Projection.InputDigest != c.InputDigest || c.Projection.Execution.Branch != c.Branch || c.Branch == "" || c.Projection.Execution.Workspace == "" || c.Projection.Execution.SessionID == "" || c.Projection.PR == nil {
		return provider, executionmodel.TaskDeliveryEvidence{}, nativeErr("merge_state_unknown", "candidate lacks its verified Run/workspace/session/PR binding")
	}
	f := c.Projection.PR
	if started.IsZero() || f.CheckedAt.Before(started) || f.QueryError != "" || !f.Merged || f.State != "merged" || f.MergedAt == nil || f.MergedAt.IsZero() || f.MergeCommit == nil || !taskmodel.ValidCommit(*f.MergeCommit) {
		return provider, executionmodel.TaskDeliveryEvidence{}, nativeErr("merge_state_unknown", "PR requires a fresh complete provider-confirmed merge")
	}
	if f.URL != c.PRURL || f.Provider != provider.Kind || f.Repository != provider.Repository || f.HeadRepository != provider.Repository || f.BaseBranch != branch || input.Repository.BaseBranch != branch || f.HeadBranch != c.Branch {
		return provider, executionmodel.TaskDeliveryEvidence{}, nativeErr("immutable_task", "PR differs from the frozen candidate target")
	}
	number, err := provider.PRNumber(f.URL)
	if err != nil || number != f.Number {
		return provider, executionmodel.TaskDeliveryEvidence{}, nativeErr("immutable_task", "PR number or provider identity changed")
	}
	return provider, executionmodel.TaskDeliveryEvidence{PRInstance: strings.TrimSuffix(provider.BaseURL, "/"), URL: f.URL, Repository: f.Repository, PRNumber: int64(number), BaseBranch: branch, State: "merged", Merged: true, MergedAt: *f.MergedAt, MergeCommit: *f.MergeCommit}, nil
}

func (tx *sqliteTx) completeRunnerDeliveryCandidate(project, taskID string) error {
	var selected string
	if err := tx.tx.QueryRowContext(tx.ctx, `SELECT selected_workflow_run_id FROM task_deliveries WHERE project_identity=? AND task_id=?`, project, taskID).Scan(&selected); err != nil {
		return err
	}
	c, err := readRunnerCandidate(tx.ctx, tx.tx, project, selected)
	if err != nil {
		return err
	}
	if c.TaskID != taskID {
		return nativeErr("immutable_task", "delivery belongs to another candidate task")
	}
	c.Participation = "completed"
	c.StopRequested = true
	c.InputsRevoked = false
	c.Revision++
	for i := range c.Actions {
		if c.Actions[i].State == "pending" {
			c.Actions[i].State = "cancelled"
		}
	}
	body, _ := json.Marshal(c)
	result, err := tx.tx.ExecContext(tx.ctx, `UPDATE runner_candidates SET revision=?,payload=? WHERE project_identity=? AND candidate_id=? AND revision=?`, c.Revision, body, project, c.ID, c.Revision-1)
	return requireIdentityWrite(result, err, "stale_version: delivered candidate changed")
}
