package execution

import (
	"context"
	"encoding/json"
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"strings"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func participatingCandidates(workflows []executionmodel.WorkflowRunRecord, taskID string) int {
	count := 0
	for _, w := range workflows {
		if w.TaskID == taskID && w.Abandonment == nil && w.Phase != "completed" && w.Phase != "abandoned" {
			count++
		}
	}
	return count
}

// selectedID="" requires every known PR to be verified unmerged. Delivery
// requires exactly the selected eligible PR merged, with all siblings known.
func verifyKnownCandidatePRs(ctx context.Context, config Config, snapshot storageports.TaskCandidateSnapshot, taskID string, verifier TaskDeliveryVerifier, selectedID string) ([]executionmodel.VerifiedCandidatePR, error) {
	seen := map[string]string{}
	results := []executionmodel.VerifiedCandidatePR{}
	merged := 0
	for _, w := range snapshot.WorkflowRuns {
		if w.TaskID != taskID {
			continue
		}
		urls := append(append([]string(nil), w.PRHistory...), w.PRURL)
		if w.PRCandidateURL != "" && w.PRCandidateURL != w.PRURL {
			urls = append(urls, w.PRCandidateURL)
		}
		fixed, err := fixedWorkflowPublicationConfig(config, w)
		if err != nil {
			return nil, err
		}
		for _, url := range uniqueSorted(urls) {
			if owner, exists := seen[url]; exists {
				return nil, nativeErr("inconsistent_delivery", "PR is bound to more than one candidate: "+owner+" / "+w.WorkflowRunID)
			}
			seen[url] = w.WorkflowRunID
			query := verifier
			if client, ok := query.(*publication.Client); ok && !publication.ProviderIdentity(publication.Config(fixed.PR)).Matches(client.Config) {
				return nil, nativeErr("authority_mismatch", "known PR verifier differs from candidate's frozen provider")
			}
			if query == nil {
				query = publicationClient(fixed)
			}
			info, err := query.Query(ctx, url)
			if err != nil || info.State != "open" && info.State != "closed_unmerged" && info.State != "merged" {
				return nil, nativeErr("merge_state_unknown", "known candidate PR could not be verified: "+url)
			}
			if (info.State == "merged") != info.Merged || info.Merged && (info.MergedAt == nil || info.MergedAt.IsZero() || info.MergeCommit == nil || !taskmodel.ValidCommit(*info.MergeCommit)) {
				return nil, nativeErr("merge_state_unknown", "known candidate PR merge evidence is incomplete or inconsistent: "+url)
			}
			_, e, err := selectedNativeDeliveryState(snapshot, taskID, w.WorkflowRunID)
			if err != nil {
				return nil, err
			}
			if err := publication.ValidateWorkerPRIdentity(info, url, fixed.PR.Repository, w.Branch, e.Workspace.Branch); err != nil {
				return nil, nativeErr("merge_state_unknown", "known PR differs from its fixed candidate branch: "+url)
			}
			results = append(results, executionmodel.VerifiedCandidatePR{w.WorkflowRunID, url, info})
			if info.Merged || info.State == "merged" {
				merged++
				if selectedID == "" {
					return results, nativeErr("already_merged", "a known task PR is merged; verify task delivery")
				}
				if w.Abandonment != nil || w.Phase == "abandoned" || w.WorkflowRunID != selectedID || url != w.PRURL {
					return results, nativeErr("inconsistent_delivery", "abandoned, historical or competing candidate PR merged; manual reconciliation is required")
				}
			}
		}
	}
	if selectedID != "" && merged != 1 {
		return results, nativeErr("task_not_ready", "selected candidate has no unique verified merged PR")
	}
	return results, nil
}

type CandidateAbandonOptions struct {
	ControlRequestDigest     string `json:"control_request_digest,omitempty"`
	TaskID                   string `json:"task_id"`
	WorkflowRunID            string `json:"workflow_run_id"`
	RequestID                string `json:"request_id"`
	ExpectedTaskVersion      uint64 `json:"expected_task_version"`
	ExpectedCandidateVersion uint64 `json:"expected_candidate_version"`
	RequestedBy              string `json:"requested_by"`
	Reason                   string `json:"reason"`
}

type CandidateAbandonReceipt struct {
	RequestID     string                               `json:"request_id"`
	TaskID        string                               `json:"task_id"`
	WorkflowRunID string                               `json:"workflow_run_id"`
	Status        string                               `json:"status"`
	Code          string                               `json:"code,omitempty"`
	Message       string                               `json:"message,omitempty"`
	Abandonment   *executionmodel.CandidateAbandonment `json:"abandonment,omitempty"`
}

func (c CommandService) AbandonNativeCandidate(ctx context.Context, config Config, options CandidateAbandonOptions) (CandidateAbandonReceipt, error) {
	if c.Store == nil || c.Project == "" || config.Identity() != c.Project {
		return CandidateAbandonReceipt{}, nativeErr("authority_mismatch", "candidate exit requires its native project")
	}
	if taskmodel.ValidateID(options.TaskID) != nil || !controlIDPattern.MatchString(options.RequestID) || options.WorkflowRunID == "" || options.ExpectedTaskVersion == 0 || options.ExpectedCandidateVersion == 0 || strings.TrimSpace(options.RequestedBy) == "" || strings.TrimSpace(options.Reason) == "" || len(options.Reason) > 4000 {
		return CandidateAbandonReceipt{}, nativeErr("invalid_batch", "candidate exit requires exact task/candidate versions, request, requester and reason")
	}
	body, err := taskmodel.CanonicalJSON(mustJSON(options))
	if err != nil {
		return CandidateAbandonReceipt{}, err
	}
	var saved storageports.CommandRecord
	if old, found, err := c.Store.LookupCommand(ctx, c.Project, options.RequestID); err != nil {
		return CandidateAbandonReceipt{}, err
	} else if found {
		if old.Kind != "abandon_candidate" || old.TargetID != options.WorkflowRunID || string(old.Payload) != string(body) {
			return CandidateAbandonReceipt{}, nativeErr("idempotency_conflict", "candidate exit request content changed")
		}
		if old.Status == "accepted" || old.Status == "rejected" {
			var r CandidateAbandonReceipt
			err := json.Unmarshal(old.Result, &r)
			return r, err
		}
		saved = old
	}
	snapshot, err := readCommandSnapshot(ctx, c.Store, c.Project, options.TaskID)
	if err != nil {
		return CandidateAbandonReceipt{}, err
	}
	taskValue, taskFound := snapshot.Graph.ByID[options.TaskID]
	var task *taskmodel.TaskRecord
	if taskFound {
		task = &taskValue
	}
	workflow, _, err := selectedNativeDeliveryState(snapshot, options.TaskID, options.WorkflowRunID)
	if err != nil {
		return CandidateAbandonReceipt{}, err
	}
	if task != nil && task.Status == "closed" {
		return CandidateAbandonReceipt{}, nativeErr("already_merged", "delivered task cannot abandon candidates")
	}
	if task == nil || task.Version != options.ExpectedTaskVersion || saved.CommandID == "" && workflow.ControlRevision != options.ExpectedCandidateVersion {
		return CandidateAbandonReceipt{}, nativeErr("stale_version", "task or candidate version changed")
	}
	if task.Status == "closed" {
		return CandidateAbandonReceipt{}, nativeErr("already_merged", "delivered task cannot abandon candidates")
	}
	if task.Status != "in_progress" || workflow.Abandonment != nil || workflow.Phase == "completed" {
		return CandidateAbandonReceipt{}, nativeErr("immutable_task", "candidate cannot exit this experiment")
	}
	if saved.CommandID == "" {
		saved = storageports.CommandRecord{ProjectIdentity: c.Project, CommandID: options.RequestID, IdempotencyKey: options.RequestID, Kind: "abandon_candidate", TargetID: options.WorkflowRunID, ExpectedVersion: options.ExpectedCandidateVersion, RequestedBy: options.RequestedBy, Payload: body, Status: "in_flight", CreatedAt: now()}
		var made bool
		saved, made, err = c.Store.EnqueueCommand(ctx, saved)
		if err != nil {
			return CandidateAbandonReceipt{}, err
		}
		if !made && string(saved.Payload) != string(body) {
			return CandidateAbandonReceipt{}, nativeErr("idempotency_conflict", "candidate exit request was reused")
		}
	}
	_, verificationErr := verifyKnownCandidatePRs(ctx, config, snapshot, options.TaskID, nil, "")
	receipt := CandidateAbandonReceipt{RequestID: options.RequestID, TaskID: options.TaskID, WorkflowRunID: options.WorkflowRunID, Status: "abandoned"}
	status := "accepted"
	if verificationErr != nil {
		var native *taskmodel.Error
		if !errors.As(verificationErr, &native) {
			return receipt, verificationErr
		}
		receipt.Code, receipt.Message = native.Code, native.Message
		receipt.Status, status = "pending_verification", "pending_verification"
		if native.Code == "already_merged" {
			receipt.Status, status = "rejected", "rejected"
		}
	}
	err = c.Store.TransactCandidate(ctx, func(tx storageports.CandidateTx) error {
		old, found, err := tx.LookupCommand(ctx, c.Project, options.RequestID)
		if err != nil || !found {
			return errors.Join(err, errors.New("candidate exit intent disappeared"))
		}
		if old.Status == "accepted" || old.Status == "rejected" {
			return json.Unmarshal(old.Result, &receipt)
		}
		current, err := tx.ReadTaskCandidateSnapshot(ctx, c.Project, options.TaskID)
		if err != nil {
			return err
		}
		currentTaskValue, currentTaskFound := current.Graph.ByID[options.TaskID]
		var currentTask *taskmodel.TaskRecord
		if currentTaskFound {
			currentTask = &currentTaskValue
		}
		w, _, err := selectedNativeDeliveryState(current, options.TaskID, options.WorkflowRunID)
		if err != nil {
			return err
		}
		if currentTask == nil || currentTask.Version != task.Version || currentTask.Status != task.Status || w.ControlRevision != workflow.ControlRevision || executionmodel.CandidateBindings(current.WorkflowRuns, current.Executions, options.TaskID) != executionmodel.CandidateBindings(snapshot.WorkflowRuns, snapshot.Executions, options.TaskID) {
			return nativeErr("stale_version", "task, candidate or PR set changed during exit verification")
		}
		if status == "accepted" {
			w.Abandonment = &executionmodel.CandidateAbandonment{RequestID: options.RequestID, RequestedBy: options.RequestedBy, Reason: options.Reason, At: now()}
			w.UpdatedAt = w.Abandonment.At
			receipt.Abandonment = w.Abandonment
			if err := tx.PutWorkflowRun(w); err != nil {
				return err
			}
		}
		old.Status, old.Result = status, mustJSON(receipt)
		if err := tx.UpdateCommand(old); err != nil {
			return err
		}
		op := executionmodel.OperationRecord{ProjectIdentity: c.Project, OperationID: options.RequestID, IdempotencyKey: options.RequestID, Kind: "abandon_candidate", Status: "confirmed", PayloadDigest: operationDigest(body), Request: body, Receipt: mustJSON(receipt), CreatedAt: saved.CreatedAt, UpdatedAt: now()}
		if status == "pending_verification" {
			op.Status = "outcome_unknown"
		}
		if status == "rejected" {
			op.Status = "rejected"
		}
		if _, made, err := tx.PutOperation(op); err != nil {
			return err
		} else if !made {
			if err := tx.UpdateOperation(op); err != nil {
				return err
			}
		}
		_, err = tx.AppendEvent(storageports.DomainEvent{ProjectIdentity: c.Project, TaskID: options.TaskID, WorkflowRunID: options.WorkflowRunID, ExternalSource: "candidate-command", ExternalID: options.RequestID + ":" + status, Type: "candidate.exit." + status, ReceivedAt: time.Now().UTC(), Payload: mustJSON(receipt)})
		return err
	})
	return receipt, err
}

func (s *Service) candidateBusinessClosed(run *reconstructiontypes.RunView) bool {
	if run == nil {
		return true
	}
	w := s.workflowRecord(run)
	return w == nil || w.Abandonment != nil || w.Phase == "abandoned" || w.Phase == "completed" || run.TaskStatus == "closed" || run.TaskStatus == "discarded"
}

func (c CommandService) recordCandidateMergeConflict(ctx context.Context, snapshot storageports.TaskCandidateSnapshot, taskID, workflowID, reason string, proofs ...executionmodel.VerifiedCandidatePR) error {
	err := c.Store.TransactCandidate(ctx, func(tx storageports.CandidateTx) error {
		_, err := tx.AppendEvent(storageports.DomainEvent{ProjectIdentity: c.Project, TaskID: taskID, WorkflowRunID: workflowID, ExternalSource: "candidate-delivery", ExternalID: taskID + ":" + executionmodel.CandidateBindings(snapshot.WorkflowRuns, snapshot.Executions, taskID) + ":" + digestValue(reason), Type: "task.delivery.conflict", ReceivedAt: now(), Payload: mustJSON(map[string]any{"reason": reason, "known_prs": proofs})})
		return err
	})
	if err != nil {
		return err
	}
	return nativeErr("inconsistent_delivery", reason)
}
