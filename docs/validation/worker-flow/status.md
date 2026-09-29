# Worker-flow validation status

Date: 2026-09-29. This implementation branch is **in progress**. The current
delivery scope is **local Codex only**. Orca local and remote code and probes
are retained but are not delivered; dispatch rejects both before claim. No
SteerLine production service or Task database was switched to the new format.

| Requirement | Current evidence | Status |
| --- | --- | --- |
| WF1 | A v1 claim without `project_steps` and merged delivery without step gate results pass isolated SQLite tests. `LoadConfig` and `RunConfiguredService` now refuse the old production protocol; CLI retry/mark-stopped/task reopen entries were removed. Historical internal step code remains compiled for existing tests but is not reachable through production configuration. | Production gate removal implemented; final gate pending |
| WF2 | Local Codex CLI preserved a native session. The [alternate Windows host service test](windows-first-feedback-service-test.json) passed first launch plus one feedback with fixed Task, WorkflowRun, Execution, session and worktree. Orca probes remain exploratory evidence outside this delivery. | Required local test passed |
| WF3 | The alternate host service test passed stopped turn without PR → pending Decision. Separate first-feedback cases delivered pure text and option-plus-note intact to the mock worker. The separate failure simulations were deferred by the revised Spec. | Required cases passed |
| WF4 | The alternate host service test committed after first turn and confirmed feedback kept HEAD. | Required test passed |
| WF5 | Unity, `.meta` and compiler-error simulations were deferred. The current production protocol rejects project verify/review, repair rounds and dirty ignore settings; delivery does not depend on their results. | Test deferred; implementation gate removed |
| WF6 | Stable answer Command IDs and formatting have unit tests. The [service test](windows-first-feedback-service-test.json) confirms pure text and option-plus-note delivery after restart, optional feedback while a PR awaits merge, and no resend from an unknown receipt. The console now exposes the worker-flow Decision and hides obsolete retry actions; CLI/UI live submission remains to check. | Partial |
| WF7 | PR head repository/branch validation passes unit tests. Isolated SQLite plus fake provider rejects a wrong repository, base and task branch before delivery. Real provider-to-service association remains. | Partial |
| WF8 | Isolated SQLite plus fake provider rejects closed-unmerged PRs, commits one delivery from a verified merge, and unlocks the successor; duplicate completion reuses that delivery. Complete service/late-event chain remains. | Partial |
| WF9 | The [Windows host probe](windows-local-host-probe.json) rejected this calling host; the [alternate-host adapter probe](windows-local-detached-probe.json) passed outside its Orca Job Object. The [Windows service test](windows-first-feedback-service-test.json) restarted at a pending Decision and continued the original local session. The Orca local restart result is retained as exploratory evidence. | Required local restart passed |
| WF10 | One [lost-feedback-receipt deduplication test](windows-first-feedback-service-test.json) passed. The broader restart matrix was deferred. | Required test passed; matrix deferred |
| WF11 | Current schema v6 service restarted at a pending Decision and continued the same session in the [Windows](windows-first-feedback-service-test.json) and [Orca local](orca-local-service-test.json) service tests. Old configuration is rejected by `LoadConfig` and the formal service entry; earlier v5 projection rejection test passed. Further old-format testing was deferred by the user. | Current-format test passed; old-format testing deferred |
| WF12 | The [Windows local service test](windows-first-feedback-service-test.json) passed on `192.168.0.56`. The [Orca local center experiment](orca-local-service-test.json), [real Codex adapter probe](orca-worker-adapter-probe.json), and prior [local](orca-local-session-probe.json)/[remote](orca-remote-session-probe.json) probes are retained. A longer Unicode Orca note still receives an extra UTF-8 BOM; the user deferred this issue and both Orca functions, so dispatch rejects them. One true local operator merge/DAG chain remains pre-release acceptance. | Local center smoke passed; both Orca functions deferred with known issue; merge chain pre-release |

PR #115 and PR #116 were closed without merging. The branch was rebased onto
`origin/main` at `63c3fa4`; focused worker-flow and console tests plus `go vet
./...` passed after that rebase.

The required `go test ./... -count=1 -timeout=45m -p 1 -parallel 1` passed before
the rebase (the `internal/steerline` package took 915.193 seconds). `go vet
./...`, Windows `go build ./...` and Linux `GOOS=linux GOARCH=amd64 go build
./...` also passed. These were earlier code checks, not final candidate gates or
WF1–WF12 acceptance. Per the revised Spec, run each complete gate once on the
final implementation candidate. Do not report all WF1–WF12 as passed.
