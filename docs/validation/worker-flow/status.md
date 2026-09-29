# Worker-flow validation status

Date: 2026-09-29. This implementation branch is **in progress**. It is not a
release candidate and no SteerLine production service or Task database was
switched to the new format.

| Requirement | Current evidence | Status |
| --- | --- | --- |
| WF1 | A v1 claim without `project_steps` and a merged-delivery transaction without step gate results pass isolated SQLite tests. The old production config path and other backends remain. | Incomplete |
| WF2 | Local Codex CLI and both Orca platform probes preserved a native session. The [alternate Windows host service test](windows-first-feedback-service-test.json) passed first launch plus one feedback with fixed Task, WorkflowRun, Execution, session and worktree. | Required test passed; center Orca integration pending |
| WF3 | The alternate host service test passed stopped turn without PR → pending Decision. Separate first-feedback cases delivered pure text and option-plus-note intact to the mock worker. The separate failure simulations were deferred by the revised Spec. | Required cases passed |
| WF4 | The alternate host service test committed after first turn and confirmed feedback kept HEAD. | Required test passed |
| WF5 | Unity, `.meta` and compiler-error simulations were deferred by the revised Spec. Removing center gates remains implementation work. | Test deferred |
| WF6 | Stable answer Command IDs and formatting have unit tests. The [service test](windows-first-feedback-service-test.json) confirms pure text and option-plus-note delivery after restart, optional feedback while a PR awaits merge, and no resend from an unknown receipt. The console now exposes the worker-flow Decision and hides obsolete retry actions; CLI/UI live submission remains to check. | Partial |
| WF7 | PR head repository/branch validation passes unit tests. Isolated SQLite plus fake provider rejects a wrong repository, base and task branch before delivery. Real provider-to-service association remains. | Partial |
| WF8 | Isolated SQLite plus fake provider rejects closed-unmerged PRs, commits one delivery from a verified merge, and unlocks the successor; duplicate completion reuses that delivery. Complete service/late-event chain remains. | Partial |
| WF9 | Local journal/process reconciliation code exists. The [Windows host probe](windows-local-host-probe.json) rejected this calling host; the [alternate-host adapter probe](windows-local-detached-probe.json) passed exact-session continuation outside its Orca Job Object. The [service test](windows-first-feedback-service-test.json) restarted at a pending Decision and continued the original session. Restart during an active turn and Orca reattachment remain unverified. | Partial |
| WF10 | One [lost-feedback-receipt deduplication test](windows-first-feedback-service-test.json) passed. The broader restart matrix was deferred. | Required test passed; matrix deferred |
| WF11 | Current schema v6 service restarted at a pending Decision and then continued the same session in the [service test](windows-first-feedback-service-test.json). Earlier v5 projection rejection test passed, but further old-format validation was deferred. `LoadConfig` still accepts old minimal-v1 configurations, so the implementation protection is incomplete. | Current-format test passed; implementation incomplete; old-format testing deferred |
| WF12 | Existing [Orca local](orca-local-session-probe.json) and [remote](orca-remote-session-probe.json) platform probes show same-session feedback; the [Windows local adapter probe](windows-local-detached-probe.json) and [service test](windows-first-feedback-service-test.json) passed on `192.168.0.56`. One true operator merge/DAG chain is reserved for pre-release acceptance; three full backend chains were deferred. | Center Orca smoke pending; merge chain deferred to pre-release |

PR #115 and PR #116 were closed without merging. The branch was rebased onto
`origin/main` at `63c3fa4`; focused worker-flow and console tests plus `go vet
./...` passed after that rebase.

The required `go test ./... -count=1 -timeout=45m -p 1 -parallel 1` passed before
the rebase (the `internal/steerline` package took 915.193 seconds). `go vet
./...`, Windows `go build ./...` and Linux `GOOS=linux GOARCH=amd64 go build
./...` also passed. These were earlier code checks, not final candidate gates or
WF1–WF12 acceptance. Per the revised Spec, run each complete gate once on the
final implementation candidate. Do not report all WF1–WF12 as passed.
