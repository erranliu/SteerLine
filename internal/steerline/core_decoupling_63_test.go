package steerline

import "testing"

func TestCD63FrozenConfigAndBinding(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(*testing.T)
	}{
		{"fixed_profile_integrity", TestProjectPolicyFrozenSnapshotRejectsDamageAndPreservesAuthority},
		{"accepted_config_recheck", TestClaimRechecksAcceptedConfigFiles},
		{"resource_receipt_commit", TestAppServerLaunchMessagePersistsPolicyResources},
		{"question_route_identity", TestCD62NativeFactsAndQuestionRoute},
		{"question_route_receipt_barrier", TestCD62QuestionReceiptBarrier},
		{"profile_copy_isolation", TestWorkerFlowContextsDoNotExchangeExecutionProfiles},
		{"original_observation_survives_requirement_drift", TestResolutionAuditRequirementsDoNotBlockOriginalObservation},
		{"legacy_permission_evidence_rejected", TestProjectPolicyNativeReportsRetainOriginalSnapshotDigests},
		{"remote_missing_evidence_still_blocks_claim", TestRemotePublicationClaimRequiresPermissionEvidence},
	} {
		t.Run(test.name, test.run)
	}
}
