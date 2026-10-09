package integration

import (
	"testing"
)

func TestCD63FrozenConfigAndBinding(t *testing.T) {
	t.Run("legacy_permission_evidence_rejected", TestProjectPolicyNativeReportsRetainOriginalSnapshotDigests)
	t.Run("remote_missing_model_blocks_claim", TestRemotePublicationClaimRequiresExplicitModel)
}
