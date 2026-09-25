# BEADS-EXIT live acceptance (2026-09-25)

This record captures the isolated live-platform acceptance run for `beads-exit.spec.md`. The fixture was a new private repository without `.beads`; its center was configured with a deliberately unavailable `bd` command. The authenticated GitHub account merged the PRs listed below. No account token or credential is stored here.

Fixture repository: [SteerLineNativeExitAcceptance](https://github.com/erranliu/SteerLineNativeExitAcceptance).

## Real A → B delivery

| Run | Backend | Task | PR | Merge commit | Center result |
| --- | --- | --- | --- | --- | --- |
| Local run | Local Codex | 901 → 902 | [#1](https://github.com/erranliu/SteerLineNativeExitAcceptance/pull/1), [#2](https://github.com/erranliu/SteerLineNativeExitAcceptance/pull/2) | `99caedead7b8ddb3e1df656ea236a783b3a56560`, `5b07b0a07e08ce618471e53379e1572c13691f82` | Both merges were verified and recorded as TaskDelivery. Automatic dispatch claimed task 902 immediately after task 901 delivery, so this run does not serve as the persisted-ready snapshot. |
| Orca run | Orca terminal-v2 | 903 → 904 | [#3](https://github.com/erranliu/SteerLineNativeExitAcceptance/pull/3) | `c3f98ee644c18761f2c60c4769f822806856e7fc` | Task 903 delivery was recorded after the human merge. Task 904 remained open and `dependency_ready=true` with no unmet predecessors in manual dispatch mode, both before and after stopping and restarting the service. |

The Orca execution used the registered local repository and fixed terminal-v2 identities. The final PR was produced by the native implement/verify/publish steps. The service restart preserved task, execution and delivery identity; it did not create a second worker. The center was shut down after the post-restart snapshot.

## Backend evidence

- Local Codex: direct installed executable reported `codex-cli 0.155.0-alpha.16`. Official `steerline certify-backend` evidence enabled local start, observe, stop, workspace inspection and output reading.
- Orca: installed service reported version `1.4.209`. Certified target `orca:f6912f0cd2ce941964b60190` supports start, observe, stop, workspace inspection and output reading. An independent disposable terminal was also read, reconnected and stopped with process termination confirmed.
- The Orca task run first encountered a terminal response whose exit field was `exitCause.kind=exited`. The exact attempt stayed `outcome_unknown` with its terminal identity and saved result. After the protocol parser/reconciliation fix and service restart, the center verified that same terminal, result path and launcher contract, completed the step and continued the workflow without replacement. This live incident exercised the conservative lost-result recovery path.
- No supervised remote was enabled or claimed as accepted.

## Spec gates and limits

- **NT12:** Real local and Orca A → B PR delivery and merge verification passed. Orca 903 → 904 is the open/ready-after-restart proof; local task 902 was auto-claimed immediately, so it is not used for that snapshot.
- **NT13 / NT21-EXIT:** Native preflight and worker tests pass with Beads unavailable, reject old `bd ready` and start/finish entry points, and verify Beads environment values are absent from worker context. Native commands and TaskDelivery use the center SQLite authority.
- **NT14 / NT15:** Clean initialization is manual and paused; dispatch requires explicit control. Binding drift, Beads state, unknown schema, unsupported state files and ambiguous old state are rejected without adoption or cleanup.
- **NT16:** Tests cover retry with a new attempt on the same run/workspace, failed-attempt preservation, receipt replay, projection failure, lost Orca terminal-create response, exact terminal exit/result recovery after restart, and an injected cleanup-receipt transaction failure. The failed receipt retained the PR and workspace cleanup identities; after removing the injected failure, restart reconciliation recorded receipts and completed cleanup. Native dispatch mode/pause are stored in center SQLite and have no config-file mirror write, so a separate config-file write failure injection does not apply to this path.
- **NT17:** Local Codex and real Orca terminal-v2 task execution passed. Supervised remote remains unenabled and untested.
- **NT22-EXIT:** Isolated local tests verify exact worker stop and workspace cleanup; the PR cleanup test verifies exact unmerged PR identity, actual closed-state evidence, pending receipt preservation on transaction failure, and independent PR/workspace receipts. The real live-platform A → B run validates merge delivery, while PR-close cleanup uses an isolated provider fixture. Remote cancellation remains manual when the backend cannot prove stop.

## Reproduction evidence in source

- Full deterministic suite: `go test ./... -count=1 -timeout 20m` (captured separately in `full-go-test-final-2026-09-25.txt`).
- Key integration tests: `TestNativeLocalProjectStepsDeliverTaskAndReleaseSuccessor`, `TestNativeOrcaProjectStepsUseFixedTaskContextAndCenterDelivery`, `TestNativePreflightDoesNotRequireBeadsAndRejectsLegacyStepEntrypoints`, `TestOrcaTerminalExitCauseConfirmsCommandExit`, `TestNativePreflightAllowsItsEmptyRunLockFile`, and `TestPRCloseUsesExactGiteaIdentityAndReturnsClosedReceipt`.
