# Task graph modules verification

Task 13 was implemented on the post-#12 source at `8e5df5e6d1cceae9b7b82aa6b1653372e214d9e7`.
The pure rules package is `internal/steerline/taskgraph`; black-box CLI tests are in
`internal/steerline/integration`.

## Package boundaries

`taskgraph` owns deterministic cycle detection, topological ordering, numeric task-ID
sorting, neighborhood traversal, and depth/node/boundary-edge projection. Its inputs
are small in-memory values. The package imports only `sort` and `time`; it does not
import the parent package or touch storage, the filesystem, the clock, Git, processes,
or execution backends.

`QueryTaskGraph` reads one center snapshot, validates that every stored edge has both
endpoints, then converts that snapshot and the normalized filters into taskgraph values.
The parent package still creates DTOs and calculates readiness/blockers from that same
snapshot. `ReadyNativeTaskSnapshot` first applies the existing center-side eligibility
checks, then uses the package only to order those already-qualified candidates.
`CommandService` keeps cycle validation, transactions, delivery checks, and command
authority. Graph protocol, cursor, revision, edge direction, and the 500-node,
500-boundary-edge, and depth-10 limits are unchanged. Export publication and overwrite
protection remain in the I/O layer.

## Test ownership map

| Previous test or behavior | New owner / treatment | Coverage retained |
| --- | --- | --- |
| `TestTaskBatchStrictDecodeAndDeterministicCyclePath` | Remains in `internal/steerline` | Strict input decoding and cycle rejection through `CommandService`; deterministic cycle algorithm cases are in `taskgraph` |
| `TestStableReadyTaskOrderUsesPriorityTimeAndTaskID` | Moved to `taskgraph.TestStableTaskOrderUsesPriorityTimeAndNumericID` | Priority, creation time, dependency ordering, and numeric ID tie-breaks; ready eligibility remains in the center |
| `TestTaskGraphViewSnapshotAndBoundary` | Remains in `internal/steerline` | Same-snapshot query, blockers, direction/filter wiring, cycle rejection, and representative boundary DTO conversion |
| `TestTaskGraphLimitsAndExportPublication` large graph matrix | Large node/depth/boundary-cap cases moved to `taskgraph` | In-memory tests cover numeric ordering, branch/merge traversal, disconnection, depth 10, node limits, and the 500-edge cap; core keeps one representative query limit and the 502-task list/detail pagination check |
| `TestTaskGraphLimitsAndExportPublication` I/O assertions | Remains in `internal/steerline` | Full-export rejection, filtered JSON publication, version/title preservation, bidi escaping, and no-overwrite behavior |
| `TestTaskGraphReadCoexistsWithWriter` | Remains in `internal/steerline` | Read snapshot and concurrent writer behavior |
| `TestCLIRejectsLegacyTaskAuthorityBeforeStateInitialization` | Moved from `internal/steerline` to `integration` | Real CLI process rejects the retired authority and leaves runtime/database paths absent |
| `TestNativeTaskListCLIInitializesReadOnlyProtocolWithoutBeads` | Replaced in `integration` by `TestTaskCLIProcessUsesNativeAuthorityWithoutInitializingMissingRead` | Real CLI processes reject uninitialized graph/export reads without side effects, successfully list a persisted native project, preserve its database revision, and reject a noncanonical ID |

The integration package builds the current CLI once in `TestMain`. The two scenarios
use separate temporary directories and native state. They run serially and pass each
child process its own `STEERLINE_HOME`; no production debug API or exported private
service field was added. All deterministic tests remain in the default `go test ./...`
run.

## Daily and cross-layer commands

```powershell
go test ./internal/steerline/taskgraph
go test ./internal/steerline/integration -count=1 -timeout 2m
go test ./internal/steerline ./cmd/steerline ./internal/steerline/integration -count=1 -timeout 20m
```

The pure package command runs only in-memory rules; it does not start a subprocess or
create a database. Run the core/integration consumers after changing conversions,
query behavior, persistence, or CLI wiring. Before integration, use the complete
no-cache entry below. It preserves original JSONL and stderr and records environment,
per-package time, test outcomes, and wall time:

```powershell
go run ./cmd/test-feedback -phase taskgraph-final-full-serial -cache-condition "Go build cache warm; -count=1 disables test-result caching" -native-acceptance "Live GitHub/Codex/Orca/visible-window switches not enabled" -out docs/validation/taskgraph-modules/final-full-serial
```

## Measurement

The baseline and candidate ran on the same Windows/amd64 host with Go `1.26.4`,
20 logical CPUs, default package parallelism, and a warm Go build cache. `-count=1`
disabled test-result caching. No live GitHub, Codex, Orca, or visible-window acceptance
switch was enabled. Package durations overlap under Go's package parallelism and must
not be summed as suite wall time.

| Measure | Post-#12 baseline | Candidate | Change |
| --- | ---: | ---: | ---: |
| Full `go test ./... -json -count=1 -timeout 20m` wall | 628.134 s | 641.348 s | +13.214 s (+2.10%) |
| `internal/steerline` package | 626.296 s | 639.921 s | +13.625 s (+2.18%) |
| `cmd/steerline` package | 40.553 s | 29.421 s | -11.132 s |
| New `internal/steerline/integration` package | — | 12.220 s | One CLI build plus two isolated black-box scenarios |
| New `internal/steerline/taskgraph` package | — | 0.536 s | Serial in-memory rule matrix |
| Warm no-test build probe | 2.589 s | 4.291 s | +1.702 s, including the new test package and its one CLI build |

Baseline report: [`baseline-retry/summary.json`](baseline-retry/summary.json). Candidate
report: [`final-full-serial/summary.json`](final-full-serial/summary.json). Both full runs completed
with 0 failures and 10 skips; the candidate had 353 terminal events (343 passed, 10
skipped). The skips are the existing native/platform opt-ins and unavailable-host
checks; they are not claimed as live acceptance. The new graph package accounts for
the added deterministic tests. The 2.10% suite wall increase is below the 70.198-second
spread between the first complete-but-failed baseline attempt and its successful
unchanged-source retry; that failed attempt is timing evidence only, not a passing
baseline. The largest execution flows retain their existing run-to-run variation, and
no behavioral or duration regression remains unresolved.

The first baseline attempt is preserved at [`baseline/summary.json`](baseline/summary.json).
It completed with one failure in the existing one-second polling test
`TestLifecycleReviewChildPublishesIdentityBeforePreflight`; that test passed by itself
in 5.332 seconds, and the complete unchanged-source retry passed. The candidate full
run also passed, including the polling test. The timing difference between the first
failed run (698.332 s) and the successful retry (628.134 s) is retained as evidence of
existing suite variance.

The build probe compares the current source with `go test ./... -run=^$`; it is not a
test-coverage result. The implementation's package, fixture, and CLI-build cost is
reported explicitly instead of being presented as an algorithm speedup.

### Pure-rule parallelism check

Three warm-cache runs repeated the pure rules package ten times with `-parallel=1`
and three with `-parallel=20`. Serial wall times were 1.294, 1.028, and 1.136 seconds
(median 1.136); parallel wall times were 1.116, 1.085, and 1.168 seconds (median
1.116). Parallel was slower in two of three pairs, and its 20 ms median difference is
smaller than the 80–270 ms run spread. The tests therefore remain serial. Reports are
under `repeat-serial*` and `repeat-parallel*`.

## Validation

- `go test ./internal/steerline/taskgraph -count=1`: passed.
- `go test ./internal/steerline/integration -count=1 -timeout 2m`: passed.
- Focused taskgraph, integration, and CLI package run: passed; the retained
  `internal/steerline` query/command/CAS/ready-wiring regression set passed.
- Full no-cache `go test ./... -json -count=1 -timeout 20m`: passed; complete JSON
  summary and raw output are under [`final-full-serial/`](final-full-serial/).
- `go vet ./...`: passed.
- Windows `go build ./...`: passed.
- Linux/amd64 `go build ./...`: passed.
- `git diff --check`: passed (Git printed only its existing LF-to-CRLF normalization
  notices for Windows checkout files).
- Rule and integration tests remain serial, so this change introduces no test
  concurrency. A race check was attempted before the parallelism decision but could not
  run on this host: `CGO_ENABLED=0`, no C compiler installed, WSL has no Go installation,
  and the Docker Linux engine is not running.

The `taskgraph` tests use only in-memory values. Integration, SQLite, Git, and
host-facing tests remain serial within their package.

The first focused integration attempt used a custom `run_dir`, which the current
native configuration correctly rejects; the fixture was changed to use an isolated
`STEERLINE_HOME`, and the focused package rerun passed. A separate focused core report
records a temporary compile error from a redeclared test path; it was corrected before
the core wiring run and the final full suite. These intermediate failure logs remain
under `focused-packages/` and `core-wiring/` for an auditable development trail.
