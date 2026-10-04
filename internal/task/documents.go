package task

import (
	"context"

	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// VerifyContractFiles validates pinned committed blobs. TASK-DAG must also
// check the actual execution workspace before claim/implementation; this check
// alone is never a startup permission.
func (s Service) VerifyContractFiles(ctx context.Context, v taskmodel.TaskContract) error {
	return verifyContractFiles(ctx, s.Input, s.Documents, v)
}

func verifyContractFiles(ctx context.Context, input Input, documents Documents, v taskmodel.TaskContract) error {
	if err := (Commands{Input: input}).validateInput(); err != nil {
		return err
	}
	if documents == nil {
		return taskError("contract_mismatch", "fixed document reader is required")
	}
	if err := taskmodel.ValidateContract(v); err != nil {
		return err
	}
	target, err := documents.RepositoryTarget(ctx)
	if err != nil {
		return err
	}
	if v.Repository != target || v.Remote != input.Remote || v.Branch != input.Branch {
		return taskError("contract_mismatch", "contract repository, remote or branch differs from the configured project")
	}
	refs := []taskmodel.FileReference{v.SpecRef, v.PlanRef}
	refs = append(refs, v.CommonRefs...)
	refs = append(refs, v.AcceptanceRefs...)
	for _, ref := range refs {
		b, err := documents.ReadCommittedRegularFile(ctx, ref)
		if err != nil {
			return err
		}
		if err := taskmodel.ValidateDocumentContent(ref, b, taskmodel.IsAcceptanceReference(v.AcceptanceRefs, ref)); err != nil {
			return err
		}
	}
	return nil
}
