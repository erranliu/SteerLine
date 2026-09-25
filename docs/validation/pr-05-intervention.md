# PR 5 人工决策与恢复验证记录

Date: 2026-09-25 (Asia/Shanghai)
Base: `7b2706f` (`origin/main` at implementation start)
Environment: Windows amd64, Go 1.26.4

Status: implementation is complete and automated checks pass. Full H-A1–H-A16 acceptance is still pending the detailed Windows TTY cases and a live same-session Orca decision-application proof; those native gates are listed below.

## Checks completed

- `gofmt` on changed Go files — passed.
- `go test ./... -run '^$' -timeout 3m` — passed for all packages; confirms current source compiles.
- `go vet ./...` — passed.
- `go build ./...` — passed on Windows amd64.
- `GOOS=linux GOARCH=amd64 go build ./...` — passed.
- `git diff --check` — passed; Git reported only its configured LF-to-CRLF working-copy notices.
- Windows interactive TTY smoke: launched `steerline console -config <temporary offline fixture>`, confirmed the offline view, config path and fixed service-window guidance, then exited with `q`. No service was started and no answer/recovery command was submitted.
- Final `go test ./... -count=1 -timeout 20m` — passed: CLI 21.594s, core 914.057s, console 1.339s, remoteprotocol 0.871s, viewer 2.170s; `cmd/orca-probe` has no test files.
- PR5-focused console/recovery/decision/command/storage tests — passed; console 1.013s, core selection 17.791s.
- `go test ./internal/steerline -run '^TestOrcaDecisionResumesRun$' -count=1 -timeout 3m -v` — passed in 13.55s; answer acceptance survived service restart and issued one continuation.

## Full-suite correction history

The first complete run exposed six failures before the final run. The fixtures and assertions were corrected to bind decisions to their current execution, attempt, service projection and run revision; assert that missing application evidence records a failed/blocked decision; expect unsupported minimal-v1 local continuation to remain stopped; and provide a real PR head SHA in the recovery provider fixture. The focused rerun passed all six cases, and the final full run above passed.

## Acceptance evidence

| ID | Current result | Evidence / remaining gate |
| --- | --- | --- |
| H-A1 | Automated pass; TTY pending | Console editor tests cover twelve options, Unicode paste, multiline input, cursor/edit keys, and length rejection. Long detail and real Windows TTY interaction still need a fixture smoke. |
| H-A2 | Automated pass | Decision validation and console tests cover declared options, free-text combinations and required notes. |
| H-A3 | Automated pass | SQLite arbitration tests cover same-answer idempotency, conflicting answers, and two independent OS client processes. |
| H-A4 | Automated pass | Decision identity, revision checks and stale-draft retention are covered by service and console tests. |
| H-A5 | Automated pass; offline TTY pass | UI and CLI submit through `CommandService`; offline submission has no alternate write path. The Windows TTY smoke confirmed offline instructions; an offline pending-decision fixture is not available. |
| H-A6 | Fake-backend pass; native gate pending | Tests cover local continuation rejection, fixed binding, lost replies and restart without replay. No isolated registered Orca repository is configured in this environment; live incarnation/reply/application evidence has not been run. |
| H-A7 | Automated pass | Retry fixtures cover preserving task/run/worktree/claim and rejecting dirty, advanced, unknown, PR-bound and unsupported runs. |
| H-A8 | Automated pass | Recovery fixtures cover service remaining live and retaining another unknown slot; completed-run rollback is rejected by the shared gates. |
| H-A9 | Automated pass | Result recovery freezes input bytes and checks digest, workspace and provider PR identity before state transition. |
| H-A10 | Automated pass | Blocker recovery fixtures cover pre-claim interruption and claim-only orphan identity; unresolved external facts remain blocked. |
| H-A11 | Automated pass | Legacy sidecar state is refused without importing an answer; CLI and console use the current command path. |
| H-A12 | Automated pass | `TestOrcaDecisionResumesRun` verifies answer persistence/recovery before continuation; `TestControlReviewInterruptedDecisionRetainsUnknownReservation` and `TestOrcaLostReplyReceiptKeepsOriginalExecutionUnknown` prevent continuation replay; `TestInFlightRetryCommandIsNeverReplayedAndCanBeResolved` and `TestControlCrashBoundariesNeverReplayClaimWorktreeOrWorker` cover recovery command and worker-start replay; result recovery tests keep PR completion behind verified evidence. |
| H-A13 | Automated pass; live capability gate pending | Shared capability/eligibility tests reject local continuation and require fixed verified execution binding. No isolated registered Orca repository is configured in this environment; native Orca capability evidence for this PR is not recorded. |
| H-A14 | Automated pass; live application proof pending | Fake result and minimal-step proof tests bind decision, attempt, execution and answer digest; send/turn-start alone do not mark applied. |
| H-A15 | Automated pass; live recovery gate pending | Fake lost-response and restart tests preserve the original operation and avoid duplicate sends. Live Orca same-operation recovery has not been tested. |
| H-A16 | Automated model pass; TTY pending | Console state tests preserve waiting decisions across folds and turn changes; real Windows TTY fold/navigation smoke has not been run. |

## Remaining acceptance gates

- Use disposable fixtures for a Windows TTY smoke of long question/context, twelve options, Unicode multiline answer, stale revision, and recovery confirmation. The recorded TTY smoke covered the offline view only.
- Run the isolated native Orca continuation path only with a disposable registered repository; record whether it provides application proof. A reply/send receipt alone does not satisfy H-A14. No isolated registered repository is configured in this environment.
- `git diff --check` was rerun after the final evidence update and passed; only configured LF-to-CRLF working-copy notices were printed.
