# TASK-CORE verification record

This records the original implementation validation. The quality audit, corrections to schema/contract behavior, and current verification results are recorded in [audit.md](audit.md). PR merge/worker execution is not verified by these storage fixtures; actual platform delivery remains a downstream acceptance requirement.

Date: 2026-09-24 (Asia/Shanghai)

Repository baseline: `e9ba8051740be640945c6854c2a3da602ab27e39`

Toolchain: `go version go1.26.4 windows/amd64`

## Acceptance coverage

| Acceptance | Evidence | Result |
| --- | --- | --- |
| NT01 | `TestNativeTaskQuerySnapshotPagingPersistenceAndIsolation`: retains canonical numeric IDs (`2`, `10`), sorts numerically, scopes queries by project and rejects duplicate IDs. `TestNativeTaskCASGuardsAndDeliveryProjection`: Task IDs remain separate from WorkflowRun IDs. | Pass |
| NT02 | `TestNativeProjectInitializesOnceAndRejectsMismatchedBinding`: native authority initializes once as manual/paused, config drift and switching back to Beads are rejected. `TestNativeDiscardCASKeepsAuditAndNeverReusesID`: discard timestamp/reason persist, ID cannot be reopened/reused, no delivery is produced. `TestNativeInitializationRejectsBeadsStateAndCustomDatabaseFork` and `TestNativeInitializationRefusesExistingBeadsTaskDatabase`: legacy Beads state is not adopted. | Pass |
| NT03 | `TestTaskContractPinsCommittedBlobsAndRejectsInvalidReferences` and `TestTaskContractRejectsCommittedSymlinkReferences`: one paired Spec/Plan, shared refs, digest/canonical JSON, acceptance heading, missing blob, traversal/absolute path, committed symlink rejection and CRLF checkout verification against committed blob bytes. | Pass |
| NT21-CORE | `TestNativeTaskCASGuardsAndDeliveryProjection`: task, selected WorkflowRun/Execution and verified PR evidence close together; delivery replay is idempotent; associations survive reopening the SQLite reader. | Pass |

Protocol sample: [native-tasks-v1.sample.json](native-tasks-v1.sample.json). Interface and ownership inventory: [interfaces.md](interfaces.md).

## Commands and results

```text
go test ./internal/steerline -run '^(TestNativeProject|TestNativeTask|TestNativeDiscard|TestNativeInitialization|TestNativeDatabase|TestNativeCursor|TestSchemaOne|TestTaskContractPins|TestTaskContractRejectsCommittedSymlinkReferences)' -count=1 -v -timeout 90s
go test ./cmd/steerline -run '^TestNativeTaskListCLI' -count=1 -v -timeout 90s
go test ./internal/steerline/remoteprotocol ./internal/steerline/viewer -count=1 -timeout 90s
go vet ./...
git diff --check
go test ./... -count=1 -timeout 10m
```

The native store/contract command passed 11 tests, including the committed symlink and missing blob cases. The CLI, remote-protocol and viewer test commands passed. `go vet ./...` and `git diff --check` passed.

The Spec Plan's `go test ./... -count=1 -timeout 10m` command timed out while `TestInterruptedPreparationNeverCreatesOrRelaunches/after-create` was running. A repeat full run with `go test ./... -count=1 -timeout 15m` passed in 620.490 seconds on Windows/amd64. The first timeout remains recorded to distinguish the actual command result from the successful longer run.

## Scope note

The fixture tests write through the repository transaction interface. There is no native business command for task creation, dependency mutation, claim, discard, or delivery in this implementation. Native CLI reads never treat `dependency_ready` or an empty blocker list as available capability.
