# TASK-CORE quality audit — 2026-09-25

Audited implementation: PR #29, starting at `6295105e2ee02003862b719a8ae7a58a07357b4e`.
Toolchain: Go 1.26.4, Windows/amd64.

## Confirmed findings and corrections

| Finding | Correction | Regression evidence |
| --- | --- | --- |
| The previous implementation automatically migrated schema 1, contrary to AGENTS.md and C4's old-state rejection requirement. | Fresh schema 3 reuses the center tables. Existing schema 1/2 and unknown formats are rejected without import or mutation. | `TestSchemaOneCenterDatabaseIsRejectedWithoutMutation` compares database bytes before/after rejection. |
| A native reader checked only the schema version, so an incomplete or altered database with that version could be accepted. | Check current table, index and trigger definitions plus database identity. Native reads use the checked reader and recheck the bound project target in each snapshot. | `TestNativeAuditSchemaDefinitionMustMatchVersionStamp`, `TestNativeAuditQueryRejectsPayloadIdentityDrift`. |
| Existing-project queries acquired the repository writer lock and could fail while a writer owned the repository. Viewer task details still unconditionally called Beads. | Take the writer lock only for initialization; route native viewer details through QueryService and include sampling time. | `TestNativeAuditReadersCoexistWithWriterAndViewerUsesNative` holds the repository lock and configures an unusable bd executable. |
| Pagination decoded every task, accepted unsafe direct offsets, and could reuse a cursor against a replacement database with the same revisions. | Bound SQL pagination with an index on decimal ID order; reject negative/out-of-range offsets; bind cursors to persistent random database identity as well as revisions. Explicitly mark startup blockers unavailable. | `TestNativeAuditPaginationBoundsAndDatabaseIdentity` covers maximum integer offsets, replacement stores and corruption outside the requested page. |
| Common-reference checks rejected execution-architecture.spec.md, imposed unrequired filename/commit pairing, and allowed extra Spec/Plan documents through acceptance_refs. Windows path canonicalization also weakened remote URL identity. | Allow the established shared architecture document; preserve individually pinned references; reject extra conventional task Specs/Plans through acceptance refs; compare remote targets exactly. Reject invalid UTF-8 before canonicalization. Semantic pairing remains a specification-review responsibility. | `TestNativeAuditContractRolesAndFixedReferenceVersions` plus the existing committed-blob/CRLF/symlink tests. |
| Dependency storage accepted cycles and permitted changes/deletion of fixed edges. | SQLite triggers reject cycles and edge mutation. No ready algorithm or new business command is introduced. | `TestNativeAuditDependencyCycleRollsBackAndEdgesAreImmutable` verifies whole-transaction rollback and edge immutability. |
| Task CAS compared time.Time representations rather than instants and could reject an unchanged creation time returned in another timezone. | Use time.Time.Equal for the fixed creation instant. | `TestNativeAuditTaskCASComparesInstantsNotTimeRepresentation`. |

## Validation

The focused native/storage/schema/command regression selection and CLI/viewer tests passed. `go vet ./...` passed.

The full run used `go test ./... -json -count=1 -timeout 15m`, since the prior Windows run exceeded the Plan's 10-minute deadline. This run timed out at 900.365 seconds during `TestWorkspaceTracksFreshRemoteWithoutChangingSource`. It also recorded one `TestOrcaDecisionResumesRun` failure caused by a Git command deadline, not an assertion against native task data.

The Orca decision test passed three consecutive isolated reruns with its original assertions/timeouts (41.175 seconds total); the Git timeout did not reproduce. The seven workspace tests left unfinished by the suite deadline all passed in a separate run (62.409 seconds). Across these runs, all 249 registered tests in the main package are accounted for: 240 passed and nine existing platform-dependent or opt-in cases skipped. CLI, viewer and remote-protocol packages passed. This is complete accounted coverage with a failed full-run attempt, not a claim that a single full run passed.

Commands, individual rerun names, skip names, package results and log SHA-256 values are saved in [audit-results.json](audit-results.json). `go vet ./...` and the staged diff check passed.

## Scope and evidence limits

All native records used here are isolated repository fixtures. Their saved PR evidence tests record consistency and restart behavior; it does not prove a real PR was queried or merged. TASK-DAG and BEADS-EXIT own production create/claim/discard/delivery admission and actual execution-workspace checks. `VerifyTaskContractFiles` verifies pinned committed blobs and is not startup permission.
