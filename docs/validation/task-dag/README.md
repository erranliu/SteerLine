# TASK-DAG validation record

Date: 2026-09-25 (Asia/Shanghai)

Implementation branch: `codex/task-dag` from `4c658c3`.

Toolchain: `go version go1.26.4 windows/amd64`; SQLite is the repository's bundled `modernc.org/sqlite` driver. Native DAG tests use isolated Git repositories and fresh SQLite state. Most PR-state cases use a local Gitea HTTP substitute and fixed PR verifier; the opt-in live delivery probe below exercises the real GitHub merge response in an isolated native transaction.

## Acceptance evidence

| Acceptance | Evidence | Result |
| --- | --- | --- |
| NT04 | `TestCreateTaskBatchAtomicIdempotentAndQueryable`, `TestStableReadyTaskOrderUsesPriorityTimeAndTaskID`, `TestNativeDeliveryNeedsExactMergedPRAndReleasesSuccessorOnce` cover a diamond, disconnected node, AND readiness, stable ready order and delivery release. | Pass in isolated SQLite fixtures |
| NT05 | `TestTaskBatchStrictDecodeAndDeterministicCyclePath`, `TestTaskBatchRejectsDuplicateAndCrossProjectDependencies`, `TestCreateTaskBatchAtomicIdempotentAndQueryable`, native task ID uniqueness and immutable-edge tests cover strict decoding, self/cyclic/duplicate/cross-project/missing/old-successor references and task ID reuse. | Pass in isolated fixtures |
| NT06 | `TestCreateTaskBatchAtomicIdempotentAndQueryable`, `TestCreateTaskBatchRollsBackEveryWriteWhenEdgeInsertFails`, `TestNativeTaskBatchCLIValidatesCreatesAndDiscardsFixedTaskIDs` cover same-request replay, different-content conflict, whole-transaction rollback and CLI payload delivery. | Pass in isolated fixtures |
| NT07 | `TestNativeClaimRechecksReadySlotAndPersistsReservationWithoutAppliedReceipt`, `TestNativeClaimCompetitionCreatesOnlyOneWorkflowReservation`, and the missing-delivery base test cover pause, slot, stale competing claim and predecessor readiness. | Pass in isolated Git/SQLite fixtures |
| NT08 | `TestNativeClaimRechecksReadySlotAndPersistsReservationWithoutAppliedReceipt` persists one WorkflowRun, Execution, Attempt and pending Operation, then reopens SQLite and checks request replay and slot retention. | Reservation restart passes; actual worker/workspace start boundaries remain owned by BEADS-EXIT and are not marked complete here. |
| NT09 | `TestNativeDeliveryNeedsExactMergedPRAndReleasesSuccessorOnce` covers open versus merged state, exact URL/repository/branch/head, selected gate evidence and transactional close. `TestTaskDiscardRequiresPreciseUnmergedPRAndRetainsUnknownIntent` covers Gitea open/merged/unknown responses. `TestLiveNativeTaskDeliveryProbe` uses a real merged GitHub PR, the actual Git head and a clean worktree, runs Go tests at that head, then commits TaskDelivery and checks successor release and restart replay in isolated SQLite. | Center gate and isolated live-platform delivery pass; production worker publication remains outside this Spec. |
| NT10 | `TestNativeDeliveryNeedsExactMergedPRAndReleasesSuccessorOnce` injects a delivery insert failure, checks that the successor stays blocked, retries the same delivery, checks duplicate completion, then reopens SQLite and checks readiness again. | Pass in isolated fixtures. |
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

Regression cases cover a known PR plus an unknown second publication, the saved discard checkpoint, inconsistent predecessor delivery, fixed task/workflow fields, and delivery arriving after discard. The original full-suite result above precedes these audit fixes. The eight focused internal audit tests passed in 75.695 seconds; the two native CLI tests passed in 15.315 seconds. After the final publish-result guard, its focused discard test passed in 13.205 seconds. `go vet ./...` and `git diff --check` passed on the audit tree. The acceptance continuation below records the final full-suite rerun.

## Acceptance continuation (2026-09-25)

The delivery fixture now injects a SQLite failure before TaskDelivery insertion, checks that no successor is released, then retries and reopens the database to check reconstructed readiness. The claim fixture reopens the database after reservation and checks that the original request identity and occupied execution slot survive. During this run the injected SQLite trigger exposed a misleading duplicate-PR error mapping; only the actual TaskDelivery PR-identity unique constraint is now reported as that conflict. Both focused tests passed after the correction.

Read-only live `PRClient.Query` calls through the supported `command` provider and `gh api` returned PR [#30](https://github.com/erranliu/SteerLine/pull/30) as merged into `main` at `2026-09-24T21:49:46Z` with merge commit `696406f006f905ddbc494f09b3915d9c1a0295fb`, and PR [#31](https://github.com/erranliu/SteerLine/pull/31) as open with `merged=false`. The probe used `base_url=https://github.com`, `repository=erranliu/SteerLine`, and the PR URLs as exact query identities; both results passed the client identity and state checks.

The opt-in `TestLiveNativeTaskDeliveryProbe` then cloned the repository into temporary local state at base `4c658c33db63372ea767ac48d8eebd003781e750`, checked out PR #30 head `91c933a92d669993787813c5c66fda8570b06e75` in a clean Git worktree, verified the base is its ancestor, and ran `go test ./cmd/steerline -count=1 -timeout 3m` at that head. Its isolated native Task/WorkflowRun/Execution/Attempt records used the fixed contract and that verified head. `CompleteNativeTask` queried GitHub again, stored the actual merge evidence as TaskDelivery, closed task 300, and released successor 301 at graph revision 2. Reopening SQLite preserved readiness and duplicate completion replayed the saved delivery. The workflow and result records were seeded by the probe, so this does not demonstrate production worker execution or PR cleanup.

This opt-in test was added after the captured full-suite run. With the environment switch unset, its default skip path passed; the enabled live run and `go vet ./...` passed. No production code changed after that full-suite run.

The call-site audit found no non-test caller of `ClaimNativeTask` or `CompleteNativeTask`. Discard currently records `execution_stop_manual`, `workspace_cleanup`, and `pull_request_close` identities in `cleanup_pending`; it does not produce external stop/close receipts. The Spec excludes production worker integration, but NT08's worker-start boundaries and NT22-DAG's cleanup outcomes still lack evidence. Neither is marked accepted by this validation record.

Final source SHA-256 for `internal/steerline/task_dag_test.go`: `d05218ef150fc870d15894befaa69cf8b4a1b7d7569ceae18711fe9a2a510c94`. Final validation ran on Go 1.26.4, Windows/amd64:

```text
go test ./internal/steerline -run '^(TestNativeDeliveryNeedsExactMergedPRAndReleasesSuccessorOnce|TestNativeClaimRechecksReadySlotAndPersistsReservationWithoutAppliedReceipt)$' -count=1 -timeout 4m
  PASS, 23.614s (before the additional post-delivery reopen assertion)
go test ./internal/steerline -run '^TestNativeDeliveryNeedsExactMergedPRAndReleasesSuccessorOnce$' -count=1 -timeout 3m
  PASS, 7.575s (with post-delivery reopen assertion)
go test ./... -count=1 -timeout 20m
  PASS: cmd/steerline 14.815s; internal/steerline 887.459s; remoteprotocol 0.691s; viewer 1.928s
$env:STEERLINE_LIVE_PR_ACCEPTANCE = '1'; go test ./internal/steerline -run '^TestLiveNativeTaskDeliveryProbe$' -count=1 -v -timeout 5m
  PASS, 15.896s with live PR #30 merge evidence and isolated SQLite delivery/restart replay
go vet ./...
  PASS
git diff --check
  PASS
go test -race ./internal/steerline -run '^TestNativeClaimCompetitionCreatesOnlyOneWorkflowReservation$' -count=1 -timeout 5m
  UNAVAILABLE: this Windows Go toolchain has CGO_ENABLED=0 and no gcc; -race requires cgo
```

The full Go suite, local NT04–NT07/NT10 center invariants, and an isolated NT09 live-platform delivery transaction pass. Full TASK-DAG acceptance remains open: NT08 worker-start/restart, NT11 execution-side contract proof, and NT22-DAG actual stop/PR/workspace cleanup and late-merge conflict receipts have not been demonstrated. PR #31 remains a draft; this record does not mark the Spec accepted.

The captured full-suite output is in [full-go-test-2026-09-25.txt](full-go-test-2026-09-25.txt).
The opt-in live delivery output is in [live-pr-delivery-2026-09-25.txt](live-pr-delivery-2026-09-25.txt).

## Completion gate

The local TASK-DAG implementation and verification record are ready for review. TASK-DAG deliberately does not ship production worker/skill integration or relationship UI; those remain the separate BEADS-EXIT and DAG-VIEW tasks. The isolated live GitHub delivery proves the center's merge gate and transaction with real platform evidence. It does not prove production worker behavior, external cleanup receipts, or late-merge conflict handling.
