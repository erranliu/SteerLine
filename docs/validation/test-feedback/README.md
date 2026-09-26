# Test feedback runs

Use `go run ./cmd/test-feedback` for the full deterministic suite. It invokes
`go test ./... -json -count=1 -timeout 20m`, saves the original JSON stream and
stderr, and writes an environment and per-test summary. The command returns the
test process exit code; an invalid or incomplete JSON stream returns failure.

Run selected packages or cases by putting Go test arguments after `--`:

```powershell
go run ./cmd/test-feedback -phase daily -out docs/validation/test-feedback/runs/daily -- ./internal/steerline -run '^TestControlCrashBoundariesNeverReplayClaimWorktreeOrWorker$' -count=1 -timeout 20m
go run ./cmd/test-feedback -phase package -out docs/validation/test-feedback/runs/steerline -- ./cmd/steerline ./internal/steerline ./internal/steerline/console -count=1 -timeout 20m
```

Each run directory contains:

- `go-test.jsonl`: original `go test -json` stdout, unchanged.
- `go-test.stderr.txt`: original stderr.
- `summary.json`: command, source revision and working tree, Go and machine
  details, cache/native acceptance settings, wall time, package and test
  durations, slowest tests, failures, skips, and missing-terminal or parse
  errors.

The recorded working-tree status excludes only this run's three output paths
(`go-test.jsonl`, `go-test.stderr.txt`, and `summary.json`) under `-out`. Other
changes under `docs/validation/test-feedback/`, including documentation and
prior reports, remain visible.

The summary keeps parent and child test durations as separate records. It uses
the timestamps in the JSON stream for an event-span measure and records package
durations individually. It does not add parent/child or parallel package times
to claim a suite wall time. `wall_seconds` is the outer `go test` process time;
`-build-only` separately measures the no-test package/build probe.

Use `-build-only` after source changes when you need a comparable build probe.
It runs all packages with `-run=^$ -count=1 -timeout 20m`. It does not count as
test coverage. Use `-summarize-log PATH -exit-code N` to summarize an existing
JSON log without re-running its tests.

For day-to-day changes, run the affected package and the cross-module tests that
exercise its callers. Use `-count=1` for tests that depend on child processes,
Git, SQLite crash/concurrency behavior, or outside platform state. Go's normal
cache is suitable for fast pure-rule tests. Before integrating a stable
candidate, run the default full suite once with `-count=1`.

The default run does not enable the live GitHub PR, native Codex/Orca, or visible
Windows service-window acceptance switches. The runner lists any detected
switches in the report. A skipped or unrun native acceptance remains unverified;
run it separately with its documented environment and retain its evidence.

## Handoff to #12 and #13

The current slowest default cases and fixture context are recorded in
[`results.md`](results.md). The final run measured the local native workflow at
131 seconds and the Orca workflow at 76 seconds; both are protected integration
cases, and the fixture refactor does not replace their subprocess, Git, or
workspace interactions. Their run-to-run timing variance is documented there.

For #12, start with the slow cases and behavior map in
[`coverage-map.md`](coverage-map.md), keeping fixture databases, repositories,
remotes, workspaces, ports, and simulated platforms writable per test. For #13,
use the same JSON report format and compare both package durations and test-event
wall spans on the same machine, Go version, concurrency, and cache condition.
The historical full-suite report is background only; it is not a substitute for
a new baseline after code changes.
