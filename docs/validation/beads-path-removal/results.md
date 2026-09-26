# Results

## Accepted #11 baseline

The comparison baseline is the latest full suite recorded by [Test Feedback #11](../test-feedback/results.md), on the same Windows host with `go1.26.4 windows/amd64`, `GOMAXPROCS=20`, default package parallelism and `-count=1`.

| Measure | #11 latest full suite |
| --- | ---: |
| Command | `go test ./... -json -count=1 -timeout 20m` |
| Process exit | 0 |
| Outer wall time | 1085.72 s |
| `internal/steerline` package | 1083.89 s |
| Terminal test/subtest events | 549 |
| Passed / failed / skipped | 532 / 0 / 17 |

The raw baseline and environment are retained at [#11 final report](../test-feedback/final-full-after-status-filter/summary.json). That run predates this refactor and contains the old Beads and whole-run behavior.

## #12 final run

Implementation PR: [#44](https://github.com/erranliu/SteerLine/pull/44) (`codex/beads-path-removal` → `main`), open for review.

The fresh full suite ran on the same Windows host with `go1.26.4 windows/amd64`, 20 logical CPUs, default package parallelism and `-count=1` through the #11 runner. Its invocation included an explicit note that live Codex, Orca, PR-provider, and visible-window acceptance switches were not enabled.

| Measure | #12 final full suite |
| --- | ---: |
| Command | `go test ./... -json -count=1 -timeout 20m` (via `cmd/test-feedback`) |
| Process exit | 0 |
| Outer wall time | 616.38 s |
| `internal/steerline` package | 614.42 s |
| Terminal test/subtest events | 343 |
| Passed / failed / skipped | 333 / 0 / 10 |
| Test-event span | 615.62 s |

The outer wall time is 469.34 seconds lower than #11 (43.2% lower); the `internal/steerline` package time is 469.47 seconds lower (43.3% lower). The event count is 206 lower because the Beads whole-run and result-path suites were removed; [coverage-map](coverage-map.md) lists each deleted suite and its retained native invariants. The local native A→B integration took 92.45 seconds and Orca took 56.58 seconds, compared with 131 and 76 seconds in #11. Both remain real Git/workspace/subprocess regression cases, and their timing varies between runs.

The complete event stream and machine details are retained in [full-final summary](full-final/summary.json), including [`go-test.jsonl`](full-final/go-test.jsonl) and stderr. The suite skipped 10 tests according to their existing opt-in/platform conditions. Live Codex, Orca, PR-provider, and visible-window acceptance remain unverified and are not inferred from this run.

Additional checks all passed:

| Check | Result |
| --- | --- |
| `go test ./... -run=^$` | passed |
| `go build ./...` on Windows | passed |
| `GOOS=linux GOARCH=amd64 go build ./...` | passed |
| `go vet ./...` | passed |
| `git diff --check` | passed (Git printed only line-ending conversion warnings) |
