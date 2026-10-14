package execution

import (
	"context"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func participatingCandidates(workflows []executionmodel.WorkflowRunRecord, taskID string) int {
	count := 0
	for _, w := range workflows {
		if w.TaskID == taskID && w.Status != "completed" && w.Status != "abandoned" {
			count++
		}
	}
	return count
}

// Starting another candidate or discarding the Task requires every known PR
// to be verified unmerged. Task delivery verifies only its selected PR.
func verifyKnownCandidatePRs(ctx context.Context, config Config, directory *ExecutionDirectory, processHost executionbackend.ManagedProcessHost, snapshot storageports.TaskCandidateSnapshot, taskID string, verifier TaskDeliveryVerifier) ([]executionmodel.VerifiedCandidatePR, error) {
	seen := map[string]string{}
	results := []executionmodel.VerifiedCandidatePR{}
	for _, w := range snapshot.WorkflowRuns {
		if w.TaskID != taskID {
			continue
		}
		urls := append(append([]string(nil), w.PRHistory...), w.PRURL)
		if w.PRCandidateURL != "" && w.PRCandidateURL != w.PRURL {
			urls = append(urls, w.PRCandidateURL)
		}
		fixed, err := fixedWorkflowPublicationConfig(config, w, directory)
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
				query = publicationClient(fixed, processHost)
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
			results = append(results, executionmodel.VerifiedCandidatePR{WorkflowID: w.WorkflowRunID, URL: url, Info: info})
			if info.Merged || info.State == "merged" {
				return results, nativeErr("already_merged", "a known task PR is merged; verify task delivery")
			}
		}
	}

	return results, nil
}

func (s *Service) candidateBusinessClosed(run *reconstructiontypes.RunView) bool {
	if run == nil {
		return true
	}
	w := s.workflowRecord(run)
	return w == nil || w.Status == "abandoned" || w.Status == "completed" || run.TaskStatus == "closed" || run.TaskStatus == "discarded"
}
