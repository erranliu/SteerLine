# TASK-DAG validation record

Date: 2026-09-25 (Asia/Shanghai)

Implementation branch: `codex/task-dag` from `4c658c3`.

Toolchain: `go version go1.26.4 windows/amd64`; SQLite is the repository's bundled `modernc.org/sqlite` driver. Native DAG tests use isolated Git repositories and fresh SQLite state. PR-state cases use a local Gitea HTTP substitute; delivery gate tests inject a fixed PR verifier. These exercise the center invariants and are not recorded as live forge evidence.

## Acceptance evidence

| Acceptance | Evidence | Result |
| --- | --- | --- |
| NT04 | `TestCreateTaskBatchAtomicIdempotentAndQueryable`, `TestStableReadyTaskOrderUsesPriorityTimeAndTaskID`, `TestNativeDeliveryNeedsExactMergedPRAndReleasesSuccessorOnce` cover a diamond, disconnected node, AND readiness, stable ready order and delivery release. | Pass in isolated SQLite fixtures |
| NT05 | `TestTaskBatchStrictDecodeAndDeterministicCyclePath`, `TestTaskBatchRejectsDuplicateAndCrossProjectDependencies`, `TestCreateTaskBatchAtomicIdempotentAndQueryable`, native task ID uniqueness and immutable-edge tests cover strict decoding, self/cyclic/duplicate/cross-project/missing/old-successor references and task ID reuse. | Pass in isolated fixtures |
| NT06 | `TestCreateTaskBatchAtomicIdempotentAndQueryable`, `TestCreateTaskBatchRollsBackEveryWriteWhenEdgeInsertFails`, `TestNativeTaskBatchCLIValidatesCreatesAndDiscardsFixedTaskIDs` cover same-request replay, different-content conflict, whole-transaction rollback and CLI payload delivery. | Pass in isolated fixtures |
| NT07 | `TestNativeClaimRechecksReadySlotAndPersistsReservationWithoutAppliedReceipt`, `TestNativeClaimCompetitionCreatesOnlyOneWorkflowReservation`, and the missing-delivery base test cover pause, slot, stale competing claim and predecessor readiness. | Pass in isolated Git/SQLite fixtures |
| NT08 | Claim state stores one WorkflowRun, Execution, Attempt, and pending Operation with a stable receipt. | Reservation persistence is covered; worker/workspace start and restart boundaries remain owned by BEADS-EXIT and are not marked complete here. |
| NT09 | `TestNativeDeliveryNeedsExactMergedPRAndReleasesSuccessorOnce` covers open versus merged state, exact URL/repository/branch/head, selected gate evidence and transactional close. `TestTaskDiscardRequiresPreciseUnmergedPRAndRetainsUnknownIntent` covers Gitea open/merged/unknown responses. | Center invariants pass with substitutes; no live forge merge evidence was run. |
| NT10 | Duplicate `CompleteNativeTask` returns the saved delivery without querying again or advancing graph revision; successor readiness derives from TaskDelivery. | Pass in isolated fixtures; no injected delivery-transaction failure test yet. |
| NT11 | `TestNativeClaimBlocksWhenFetchedBaseMissesPredecessorMerge` fetches the configured branch and rejects a base that omits the recorded predecessor merge. Contract blob verification runs before claim. | Pass in isolated Git fixtures; execution-side contract mismatch is not production-integrated. |
| NT22-DAG | Discard tests cover open PR, already-merged PR, unknown PR state, active execution, cleanup-pending identity, no successor release and no task ID reuse. | Pass with local PR substitute; live cancellation/cleanup receipts are not available in this phase. |
| NT21-DAG | Existing control/dispatch/retry/accept-result tests remain part of the full suite. Native claim returns `start_task_applied=false` until a production worker start is recorded. | Full suite result recorded below; native worker integration remains BEADS-EXIT scope. |

## Commands and results

```text
go test ./internal/steerline -run '^(TestTaskBatchStrictDecodeAndDeterministicCyclePath|TestCreateTaskBatchAtomicIdempotentAndQueryable|TestCreateTaskBatchRollsBackEveryWriteWhenEdgeInsertFails|TestTaskBatchRejectsDuplicateAndCrossProjectDependencies|TestStableReadyTaskOrderUsesPriorityTimeAndTaskID|TestNativeClaimRechecksReadySlotAndPersistsReservationWithoutAppliedReceipt|TestNativeClaimBlocksWhenFetchedBaseMissesPredecessorMerge|TestNativeClaimCompetitionCreatesOnlyOneWorkflowReservation|TestNativeDeliveryNeedsExactMergedPRAndReleasesSuccessorOnce|TestTaskDiscardRequestPersistsUnknownAndThenDiscardsOnlyAfterUnmergedProof|TestTaskDiscardRequiresPreciseUnmergedPRAndRetainsUnknownIntent|TestTaskDiscardDuringActiveExecutionKeepsSlotAndCleanupIdentity|TestNativeTaskQuerySnapshotPagingPersistenceAndIsolation|TestNativeDiscardCASKeepsAuditAndNeverReusesID)$' -count=1 -timeout 5m
go test ./cmd/steerline -run '^(TestNativeTaskBatchCLI|TestNativeTaskListCLI)' -count=1 -timeout 3m
go test ./... -count=1 -timeout 10m
go test ./... -count=1 -timeout 15m
go vet ./...
git diff --check
```

The focused internal tests passed in 111.232 seconds on the initial PR tree; the focused CLI tests passed in 12.599 seconds. The first full `go test ./... -count=1 -timeout 10m` timed out in the pre-existing `TestPRFailuresNeverClose/failure` workspace-preparation path, matching the long-running regression recorded by TASK-CORE. The repeat `go test ./... -count=1 -timeout 15m` passed; the `internal/steerline` package completed in 870.344 seconds, and command, remote-protocol and viewer packages passed. `go vet ./...` and `git diff --check` passed on that tree.

## Quality audit follow-up

The PR review found and fixed these center invariants:

- A known PR URL no longer clears uncertainty from another publish attempt. The preflight and transactional discard checks now share one plan, and the reconciled PR/cleanup checkpoint remains durable across a pending result.
- A closed task without delivery remains visible with `inconsistent_delivery`; successor queries and claims stay blocked. Duplicate completion also rejects an open task with a stray delivery.
- Native workflow definition, step contracts, runner settings, repository, and branch are fixed after reservation. Delivery validates the selected gate structure. Task status updates preserve `goal_ids` and the identity-map digest.
- The storage writer rejects normal delivery for a discarded task, including a late merged PR result.

Regression cases cover a known PR plus an unknown second publication, the saved discard checkpoint, inconsistent predecessor delivery, fixed task/workflow fields, and delivery arriving after discard. The original full-suite result above precedes these audit fixes. The eight focused internal audit tests passed in 75.695 seconds; the two native CLI tests passed in 15.315 seconds. After the final publish-result guard, its focused discard test passed in 13.205 seconds. `go vet ./...` and `git diff --check` passed on the audit tree. The full suite was not rerun after these fixes.

## Completion gate

The local TASK-DAG implementation and verification record are ready for review. TASK-DAG deliberately does not ship production worker/skill integration or relationship UI; those remain the separate BEADS-EXIT and DAG-VIEW tasks. The local PR substitute does not constitute live forge merge evidence, so the record does not claim that live-platform delivery/cleanup gates have passed.
