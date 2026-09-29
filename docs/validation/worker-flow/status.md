# Worker-flow validation status

Date: 2026-09-29. This implementation branch is **in progress**. It is not a
release candidate and no SteerLine production service or Task database was
switched to the new format.

| Requirement | Current evidence | Status |
| --- | --- | --- |
| WF1 | A v1 claim without `project_steps` and a merged-delivery transaction without step gate results pass isolated SQLite tests. The old production config path and other backends remain. | Incomplete |
| WF2 | Local Codex CLI, Orca local and authenticated remote Orca platform probes preserved a native session across turns. No three-feedback SteerLine service run exists. | Incomplete |
| WF3 | The new phase functions and generic waiting Decision path have unit coverage. UI, structured options and actual service flow need end-to-end tests. | Incomplete |
| WF4 | No partial-commit service continuation test. | Not run |
| WF5 | No `.meta`/compiler-error service continuation test. | Not run |
| WF6 | Stable answer Command IDs, option-plus-note formatting and turn intent code exist. No lost-receipt/restart/CLI-and-UI service test. | Incomplete |
| WF7 | PR head repository/branch validation and bad-identity unit tests pass. No real provider-to-service PR association test. | Incomplete |
| WF8 | An isolated SQLite merged delivery closes one Task and unlocks its successor without step results. No complete closed-unmerged/late-event service test. | Incomplete |
| WF9 | Local journal/process reconciliation code exists. The [Windows host probe](windows-local-host-probe.json) rejected this calling host; the [alternate-host adapter probe](windows-local-detached-probe.json) passed exact-session continuation outside its Orca Job Object. No SteerLine restart test has run there. Orca reattachment is not connected. | Incomplete |
| WF10 | No fault injection across start, feedback and delivery boundaries. | Not run |
| WF11 | Fresh runtime schema v6 reopens and v5 is rejected before worktree preparation in tests. `LoadConfig` still accepts old minimal-v1 configurations, so the old format rejection requirement is not met. | Incomplete |
| WF12 | [Orca local](orca-local-session-probe.json) and [remote](orca-remote-session-probe.json) platform probes show three accepted same-session feedback turns each. The [Windows local adapter probe](windows-local-detached-probe.json) passed on `192.168.0.56`. They contain no SteerLine delivery, operator merge or DAG evidence. | Not accepted |

PR #115 and PR #116 were closed without merging. The branch was rebased onto
`origin/main` at `63c3fa4`; focused worker-flow and console tests plus `go vet
./...` passed after that rebase.

The required `go test ./... -count=1 -timeout=45m -p 1 -parallel 1` passed before
the rebase (the `internal/steerline` package took 915.193 seconds). `go vet
./...`, Windows `go build ./...` and Linux `GOOS=linux GOARCH=amd64 go build
./...` also passed. These are code checks, not WF1–WF12 acceptance. Rerun
after completing the implementation.
