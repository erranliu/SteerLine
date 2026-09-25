# Test feedback results

## Environment and measurement method

Measurements ran on the same Windows host: ASUS Vivobook S 16 M5606WA,
AMD Ryzen AI 9 365, 10 cores / 20 logical CPUs, Go `go1.26.4 windows/amd64`,
`GOMAXPROCS=20`, default package parallelism, and the default Go build cache
(path recorded in the run metadata). The initial implementation was
commit `f363c427638de5ddee2711fe2a155cc5017484fb` with a clean source worktree.
`-count=1` disabled Go test-result caching. The fixture comparison used a warm
Go build cache for both sides; a separate `-run=^$` build probe was recorded
before and after.

No live GitHub, native Codex/Orca, or visible-window acceptance switch was
enabled. Their test names may still appear as skipped tests or default
no-ops/pass events, so the action summary below does not claim those external
acceptances passed. Their status remains **not run** for this measurement.

## Full suite before fixture changes

| Measure | Result |
| --- | ---: |
| Command | `go test ./... -json -count=1 -timeout 20m` |
| Process exit | 0 |
| Outer wall time | 990.6 s |
| Main `internal/steerline` package time | 980.34 s |
| Test / subtest terminal events | 538 |
| Passed | 521 |
| Failed | 0 |
| Skipped tests | 17 |
| Package outcomes | 5 pass, 1 package with no test files |

The skipped package was `cmd/orca-probe` (no test files). All 17 test skips are
listed in [`baseline-before-summary/summary.json`](baseline-before-summary/summary.json).
They include live GitHub/Orca probes, native Codex/Orca checks, and detached
service tests whose platform setup was unavailable. The initial raw stream and
machine record are [`baseline-before.jsonl`](baseline-before.jsonl) and
[`baseline-before.meta.json`](baseline-before.meta.json); a complete per-test
summary is [`baseline-before-summary/summary.json`](baseline-before-summary/summary.json).

The slowest cases before changes were:

| Test | Seconds |
| --- | ---: |
| `TestNativeLocalProjectStepsDeliverTaskAndReleaseSuccessor` | 98.40 |
| `TestNativeOrcaProjectStepsUseFixedTaskContextAndCenterDelivery` | 58.55 |
| `TestControlCrashBoundariesNeverReplayClaimWorktreeOrWorker` | 35.75 |
| `TestBadHandoffsNeverClose` | 34.73 |
| `TestInvalidCompletionReceiptsNeverComplete` | 32.73 |

These integration checks remain in the default suite. The first two exercise
real local process/Git/workspace flow with isolated test backends; they are not
live third-party platform acceptance.

## Measured fixture change

The paired group was:

```text
^(TestControlCrashBoundariesNeverReplayClaimWorktreeOrWorker|TestBadHandoffsNeverClose|TestInvalidCompletionReceiptsNeverComplete)$
```

It ran eight control crash boundaries, seven bad handoff variants, and five
completion receipt variants. Each run reported 23 passed test/subtest events,
zero failures, and zero skips. With the same host, command, Go version, package
parallelism, warm build cache, and `-count=1`, the outer wall times were:

| Run | Before (s) | After seed-file copy (s) |
| --- | ---: | ---: |
| 1 | [104.78](fixture-before-1-summary/summary.json) | [97.46](fixture-copy-after-1/summary.json) |
| 2 | [105.02](fixture-before-2-summary/summary.json) | [98.15](fixture-copy-after-2/summary.json) |
| 3 | [116.80](fixture-before-3-summary/summary.json) | [98.41](fixture-copy-after-3/summary.json) |
| Median | **105.02** | **98.15** |

The median fell by **6.87 seconds (6.54%)**. Per-run raw output, environment,
and summaries are in `fixture-before-1..3` and `fixture-copy-after-1..3` report
files. The selected fixture setup fell from eight Git subprocesses per case to
two per case, with eight seed-creation Git commands once per test process.
Across 20 fixtures, that is 160 Git subprocesses before and 48 after, a 70%
reduction including seed setup. The fixture also stopped rewriting, committing,
and pushing the same two starter files for every case. Each case still gets its
own copied worktree, bare remote, fake database, runtime directory, server, and
writable Git objects.

An earlier attempt using per-case `git clone` was slower and was discarded; its
exploratory reports are retained as `fixture-after-1..3` but are not included in
the accepted before/after comparison.

The warm-cache build-only probe took 3.18 seconds before and 3.25 seconds after
the implementation changes. It ran `go test ./... -run=^$ -count=1 -timeout 20m`
and is reported separately; it had no selected test events. These probes do not
claim cold-build cost or business-code speedup.

## Full suite before the status-filter boundary fix

The pre-fix complete run is stored at
[`final-full/summary.json`](final-full/summary.json), with original output in
`final-full/go-test.jsonl` and `final-full/go-test.stderr.txt`.

| Measure | Result |
| --- | ---: |
| Command | `go test ./... -json -count=1 -timeout 20m` |
| Process exit | 0 |
| Outer wall time | 908.38 s |
| Event timestamp span | 907.70 s |
| Main `internal/steerline` package time | 906.63 s |
| Test / subtest terminal events | 547 |
| Passed | 530 |
| Failed | 0 |
| Skipped | 17 |
| Package outcomes | 6 pass, 2 packages with no test files |

This run completed before the status-filter fix and added nine passing
test/subtest events for the reporter and fixture-isolation check. It recorded
82.22 seconds less wall time than the initial baseline. Its `worktree_status`
field is superseded by the post-fix report below.

## Latest full suite after the status-filter fix

The current complete run is stored at
[`final-full-after-status-filter/summary.json`](final-full-after-status-filter/summary.json),
with the original stream in `final-full-after-status-filter/go-test.jsonl`.

| Measure | Result |
| --- | ---: |
| Command | `go test ./... -json -count=1 -timeout 20m` |
| Process exit | 0 |
| Outer wall time | 1085.72 s |
| Event timestamp span | 1082.63 s |
| Main `internal/steerline` package time | 1083.89 s |
| Test / subtest terminal events | 549 |
| Passed | 532 |
| Failed | 0 |
| Skipped | 17 |
| Package outcomes | 7 pass, 1 package with no test files |

This run is 95.12 seconds slower than the 990.60-second initial baseline and
177.34 seconds slower than the preceding post-fixture full run. Its package
test source is unchanged from that preceding run apart from two new
`cmd/test-feedback` reporter tests, which account for less than one second.
The largest per-case increases are in tests that do not use the refactored
`newFixture`: `TestNativeLocalProjectStepsDeliverTaskAndReleaseSuccessor`
was 130.56 s (98.40 s initially), `TestNativeOrcaProjectStepsUseFixedTaskContextAndCenterDelivery`
was 76.10 s (58.55 s initially), and `TestCreateTaskBatchAtomicIdempotentAndQueryable`
was 42.73 s (25.71 s initially). All three use the separate
`nativeTaskFixture`; no code in that setup or those tests changed.

The measured fixture group still improves: in this full run the three parent
cases together took 100.79 s, versus 103.21 s in the initial full run, and the
three paired repetitions show a 6.54% lower median. Their setup and assertions
are unchanged from the paired measurements. This isolates the measured fixture
improvement from the run-to-run spread in the unrelated native task/process and
SQLite integration cases. The latest full-suite wall time did not improve over
baseline, so no full-suite speedup is claimed.

The latest warm-cache `-build-only` probe took 3.01 seconds
([report](build-probe-post-status-filter/summary.json)). It is separate from the
1085.72-second test run. The reporter and environment snapshot regression test
passed separately in [`status-filter-review/summary.json`](status-filter-review/summary.json).

## Native acceptance boundary

The full suite did not query fixed live GitHub PRs, connect to the registered
Orca terminal, invoke an installed Codex CLI, or open a visible service window.
The explicit switches were unset. The 17 skipped cases list the live PR,
Codex/Orca, and detached-service checks by name in the full summary. The four
visible-window acceptance tests (`TestWindowsVisibleServiceWindowAcceptance`,
`TestWindowsVisibleServiceWindowCloseAndRestart`,
`TestWindowsVisibleConversationCollectionAcrossViewerRestart`, and
`TestWindowsConcurrentVisibleServiceWindows`) return without exercising a
window when `STEERLINE_RUN_WINDOW_ACCEPTANCE` is unset, so their `pass` events
are no-op test-function exits, not acceptance evidence. Setting the switch
currently reaches tests that skip the retired legacy Beads window path. No
visible-window acceptance was performed; no-op or skipped events are not
counted as passed platform acceptance.

## Final checks

| Check | Result |
| --- | --- |
| Full suite after filter fix through `cmd/test-feedback` | Passed, exit 0 |
| Post-fix runner/summarizer tests | Passed, 2 tests |
| `go vet ./...` | Passed |
| Windows `go build ./...` | Passed |
| Linux/amd64 `go build ./...` | Passed |
| `git diff --check` | Passed (Git printed line-ending normalization notices for three edited Go test files) |

The earlier `final-full` run predates the status-filter fix. The current
`final-full-after-status-filter` report excludes only its own three output
files; its worktree snapshot includes this directory's README, coverage map,
results, and prior reports.
