# Test feedback runs

Use the quick check after ordinary code edits. It runs a real local Git/SQLite/
worker/PR delivery and restart loop, the exact merged-PR rejection case, and
every test in the pure taskgraph and remoteprotocol packages:

```powershell
go run ./cmd/test-feedback -quick -out "$env:TEMP/steerline-quick"
go run ./cmd/test-feedback -quick -out "$env:TEMP/steerline-quick-focused" -- ./internal/steerline/taskgraph -run '^TestCycleIsDeterministicAndIncludesRepeatedEndpoint$'
```

The report says `scope=quick`. Add focused tests for the changed path: native
answer application and unknown recovery for session edits; Orca contract and
simulated execution for Orca edits; CAS, idempotency and receipt failures for
SQLite or command edits; console integration for console edits. If the change
touches a new capability outside this loop, run its focused tests too. A pure
documentation edit needs identity, link, reference and semantic checks.

Run the full deterministic suite once for a stable implementation candidate
before its PR or integration. Run it earlier for changes to database format,
restart recovery, external execution identity, PR delivery or shared interfaces:

```powershell
go run ./cmd/test-feedback -workers=3 -out "$env:TEMP/steerline-full"
go run ./cmd/test-feedback -workers=1 -out "$env:TEMP/steerline-serial"
```

The first command enumerates current tests, assigns each core top-level test
once to three independent processes using fixed recorded weights, then runs all
other packages once. The second is the serial comparison and troubleshooting
entry. Both save original JSON streams and stderr and return failure for test,
coverage, process, or malformed-log errors. Reports say `scope=full`. Native
platform and visible-window acceptance still run with their own documented
entry and evidence; `-workers=3` refuses their environment switches.

Run selected packages or cases by putting Go test arguments after `--`:

```powershell
go run ./cmd/test-feedback -phase daily -out docs/validation/test-feedback/runs/daily -- ./internal/steerline -run '^TestControlCrashBoundariesNeverReplayClaimWorktreeOrWorker$' -count=1 -timeout 20m
go run ./cmd/test-feedback -phase package -out docs/validation/test-feedback/runs/steerline -- ./cmd/steerline ./internal/steerline ./internal/steerline/console -count=1 -timeout 20m
```

Each serial or targeted run directory contains:

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

The serial summary keeps parent and child test durations as separate records. It uses
the timestamps in the JSON stream for an event-span measure and records package
durations individually. It does not add parent/child or parallel package times
to claim a suite wall time. `wall_seconds` is the outer `go test` process time;
`-build-only` separately measures the no-test package/build probe.

Use `-build-only` after source changes when you need a comparable build probe.
It runs all packages with `-run=^$ -count=1 -timeout 20m`. It does not count as
test coverage. Use `-summarize-log PATH -exit-code N` to summarize an existing
JSON log without re-running its tests.

The quick and three-process runs save separate `test.jsonl` and `stderr.txt`
per group plus one `summary.json` with scope, assignments, actual group wall
times, enumerated packages, preparation and execution wall times, environment,
and coverage errors. Quick results cannot stand in for full suite or native
platform acceptance. Existing task contracts that explicitly require a full
regression or native check retain those gates.

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
