# Worker-flow validation status

Date: 2026-09-29. The table below records the original local Codex delivery
status. Local Codex app-server native ask was subsequently merged in PR #126.
The current Orca native implementation is tracked in the addendum below; the
supervised remote path remains deferred. No SteerLine production service or Task
database was switched to the new format.

| Requirement | Current evidence | Status |
| --- | --- | --- |
| WF1 | A v1 claim without `project_steps` and merged delivery without step gate results pass isolated SQLite tests. `LoadConfig` and `RunConfiguredService` refuse the old production protocol; CLI retry/mark-stopped/task reopen entries were removed. Historical internal step code remains compiled for existing tests but is not reachable through production configuration. | Production gate removal implemented; final checks passed |
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

PR #115 and PR #116 were closed without merging. This branch is based on
`origin/main` at `ec19b11`, including PR #120; the PR is mergeable. The
[final local candidate](final-local-candidate.json) at code revision `059f14b`
passed `go test ./... -count=1 -timeout=45m -p 1 -parallel 1`, `go vet ./...`,
Windows `go build ./...` and Linux `GOOS=linux GOARCH=amd64 go build ./...`.
The real operator merge/DAG chain remains a pre-release acceptance item.
Do not report all WF1–WF12 as passed.

## Local Orca native addendum

The [implementation evidence](orca-native-implementation-2026-09-29.json) and
[real center probe](orca-native-center-live-2026-09-29.json) confirm native Orca
ask/reply, SteerLine restart, two settled Task/Dispatch turns, and remembered
context on the original terminal/incarnation. SQLite fake-provider regression
uses the public answer command, confirms duplicate submission reuses its
command, and reconciles a lost Dispatch receipt without duplicate injection.
Real interactive UI/CLI answer use, Orca-specific PR
provider/merge delivery, and a second real agent remain unverified. This
addendum does not turn those items into completed ON1–ON11 acceptance.
