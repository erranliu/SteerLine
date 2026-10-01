package steerline

import "testing"

// These named entrypoints collect the existing migrated assertions without
// running their old broad packages a second time in the focused CD64 command.
func TestCD64IntentReceiptAndRecoveryGate(t *testing.T) {
	t.Run("host_wait_feedback_keeps_original_question", TestWorkerEnvironmentER2HostWaitPreservesQuestionAndPR)
	t.Run("deadline_late_result_retains_manual_gate", TestWorkerEnvironmentER4DeadlineRestartAndLostReceipt)
	t.Run("review_receipt_and_recovery_isolation", TestCD64ReviewReceiptAndRecoveryIsolation)
	t.Run("intent_precedes_dispatch", TestWorkerIntentIsDurableBeforeDispatchAndFailurePreventsDispatch)
	t.Run("receipt_failure_retains_intent", TestWorkerReceiptFailureRetainsIntentAndCancelsUnregisteredObserver)
	t.Run("partial_session_binding_survives", TestAppServerFailedPreparationKeepsPartialSessionBinding)
	t.Run("worker_commit_rolls_back", TestWorkerCommitRollsBackOnStaleRecordAndCheckpointFailure)
	t.Run("unknown_feedback_reconciles", TestAppServerPendingFeedbackRecoveryUsesNewTurnReceipt)
}

func TestCD64OriginalSessionReconciliation(t *testing.T) {
	t.Run("app_server_two_turns", TestAppServerServiceTwoTurnsPersistNativeReceipts)
	t.Run("native_original_session", TestNativeOrcaCenterReusesWorkerAfterRestartAndSettlement)
	t.Run("remote_admission_stays_blocked", TestRemotePublicationClaimRequiresPermissionEvidence)
}
