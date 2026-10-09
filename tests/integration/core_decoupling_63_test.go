package integration

import (
	"testing"
)

func TestCD63FrozenConfigAndBinding(t *testing.T) {
	t.Run("legacy_permission_evidence_rejected", TestProjectPolicyNativeReportsRetainOriginalSnapshotDigests)
	t.Run("remote_unreachable_blocks_claim", TestRemotePublicationClaimRejectsUnreachableOrca)
}
