package steerline

import "testing"

func TestCD63FrozenConfigAndBinding(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(*testing.T)
	}{
		{"fixed_profile_integrity", TestProjectPolicyFrozenSnapshotRejectsDamageAndPreservesAuthority},
		{"accepted_config_recheck", TestClaimRechecksAcceptedConfigFiles},
		{"recovery_seed_resources", testCD63RecoverySeedResources},
		{"environment_recheck_bindings", TestWorkerEnvironmentER4DeadlineRestartAndLostReceipt},
		{"resource_receipt_commit", TestAppServerLaunchMessagePersistsPolicyResources},
		{"question_route_identity", TestCD62NativeFactsAndQuestionRoute},
		{"question_route_receipt_barrier", TestCD62QuestionReceiptBarrier},
		{"profile_copy_isolation", TestWorkerFlowContextsDoNotExchangeExecutionProfiles},
		{"requirements_block_new_intents", TestResolutionAuditRequirementsBlockOnlyNewWorkerIntents},
	} {
		t.Run(test.name, test.run)
	}
}
