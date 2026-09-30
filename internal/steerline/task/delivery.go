package task

import "encoding/json"

// ValidateDelivery checks the fixed PR merge evidence before a delivery write.
func ValidateDelivery(v TaskDeliveryRecord, targetBranch string) error {
	if v.ProjectIdentity == "" || v.TaskID == "" || v.SelectedWorkflowRunID == "" || v.PRInstance == "" || v.PRRepository == "" || v.PRNumber <= 0 || v.PRURL == "" || !ValidCommit(v.MergeCommit) || v.MergedAt.IsZero() || v.RecordedAt.IsZero() {
		return newError("inconsistent_delivery", "native task delivery has incomplete fixed identity")
	}
	if len(v.Evidence) == 0 || !json.Valid(v.Evidence) {
		return newError("inconsistent_delivery", "native task delivery has no readable PR merge evidence")
	}
	var evidence TaskDeliveryEvidence
	if err := json.Unmarshal(v.Evidence, &evidence); err != nil {
		return newError("inconsistent_delivery", "native task delivery PR merge evidence has an invalid shape")
	}
	if !evidence.Merged || evidence.State != "merged" || evidence.PRInstance != v.PRInstance || evidence.URL != v.PRURL || evidence.Repository != v.PRRepository || evidence.PRNumber != v.PRNumber || evidence.MergeCommit != v.MergeCommit || !evidence.MergedAt.Equal(v.MergedAt) || evidence.BaseBranch != targetBranch {
		return newError("inconsistent_delivery", "native task delivery PR evidence differs from its fixed identity")
	}
	if evidence.ExistingAcceptance != nil {
		return newError("unsupported_schema", "old acceptance delivery is unsupported")
	}
	return nil
}
