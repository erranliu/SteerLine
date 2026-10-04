package execution

import (
	contracts "github.com/erranliu/SteerLine/internal/steerline/contracts"
)

import (
	"context"

	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// Retired candidates remain PR observation sources. Their provider facts are
// audit records and cannot replace the selected TaskDelivery.
func (s *Service) pollRetiredCandidatePRs(ctx context.Context, run *contracts.Run, fixed workerFlowContext) error {
	w, e, c := fixed.workflow, fixed.execution, fixed.config
	urls := uniqueSorted(append(append(append([]string(nil), w.PRHistory...), w.PRURL), w.PRCandidateURL))
	for _, url := range urls {
		info, err := publicationClient(c).Query(ctx, url)
		if err != nil {
			continue
		}
		if err := publication.ValidateWorkerPRIdentity(info, url, c.PR.Repository, w.Branch, e.Workspace.Branch); err != nil {
			continue
		}
		if w.Phase != "completed" && (url == w.PRURL || w.PRURL == "") {
			w.PRURL, w.PRState, w.PRMergedAt, w.PRMergeCommit, w.PRQueriedAt = info.URL, info.State, info.MergedAt, info.MergeCommit, ptr(now())
			s.refreshRun(run)
			if err := s.commitWorker(run); err != nil {
				return err
			}
		}
		typeName := "candidate.pr.observed"
		if info.Merged {
			winner := false
			for _, d := range s.domain.Deliveries {
				if d.TaskID == run.TaskID && d.SelectedWorkflowRunID == run.RunID && d.PRURL == info.URL {
					winner = true
				}
			}
			if !winner {
				typeName = "task.delivery.conflict"
			}
		}
		err = s.commandStore.TransactCandidate(ctx, func(tx persistence.CandidateTx) error {
			_, err := tx.AppendEvent(storageports.DomainEvent{ProjectIdentity: c.identity, TaskID: run.TaskID, WorkflowRunID: run.RunID, ExecutionID: e.ExecutionID, AttemptID: run.AttemptID, ExternalSource: "candidate-pr", ExternalID: url + ":" + info.State + ":" + deref(info.MergeCommit), Type: typeName, ReceivedAt: now(), Payload: mustJSON(info)})
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}
