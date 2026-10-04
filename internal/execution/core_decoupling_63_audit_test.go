package execution

import (
	"context"
	"testing"
)

func testCD63RecoverySeedResources(t *testing.T) {
	s, run := neutralAuditService(t)
	workflow, record := s.workflowRecords(run)
	record.Process, record.PolicyResources = nil, nil
	record.Status, record.RecoveryState, record.RecoveryRequired = "pending", "resuming", false
	record.Generation, record.ConfigurationDigest = newID(), workflow.ExecutionProfile.Digest
	record.CurrentTurnID, record.PendingTurnID = "", ""
	removeImplementationEnvelope(record, neutralBindingKey)
	setNeutralBinding(t, record, "", "", workerFlowMockSession)
	s.refreshRun(run)
	if _, err := s.executionConfigForRun(run); err != nil {
		t.Fatalf("original thread recovery seed was rejected before resource binding: %v", err)
	}
	require(t, s.commitWorker(run))
	require(t, checkFixtureStoredExecutions(context.Background(), s.Config))
	acceptedDigest := record.ConfigurationDigest
	record.ConfigurationDigest = "other-accepted-snapshot"
	setNeutralBinding(t, record, "", "", workerFlowMockSession)
	if _, err := s.executionConfigForRun(run); err == nil {
		t.Fatal("recovery seed from another fixed configuration was accepted")
	}
	record.ConfigurationDigest = acceptedDigest
	setNeutralBinding(t, record, "", "", workerFlowMockSession)
	record.RecoveryState = "interrupted"
	if _, err := s.executionConfigForRun(run); err == nil {
		t.Fatal("missing original resource receipt was accepted outside pending recovery")
	}
}
