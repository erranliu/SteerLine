# #81 Testkit and integration ownership

Starting source: [`e3ef412fd9bd00aab2b8f06c62d97686f937b44d`](https://github.com/erranliu/SteerLine/commit/e3ef412fd9bd00aab2b8f06c62d97686f937b44d), the merge commit for [#80 / PR225](https://github.com/erranliu/SteerLine/pull/225). The previous task's final implementation head was `f4bc42ba1b345ae9f46abe06dc3d75ce61ed7d5d`; PR225 merged at `2026-10-06T05:53:10Z`. This task starts after that merge and does not repeat its moves.

## Source and test owners

| Before | Current owner | Files and reason |
| --- | --- | --- |
| `internal/coreboundary` | [`internal/testkit/boundary`](../../../internal/testkit/boundary) | [`check.go`](../../../internal/testkit/boundary/check.go) owns all-tag source scanning and Windows/Linux production dependency checks. [`execution_boundary_test.go`](../../../internal/testkit/boundary/execution_boundary_test.go) keeps the existing TM04/05/06/07/08 negative fixtures and host-closure checks with their support owner. |
| `internal/testboundary` | [`internal/testkit/boundary/e2e`](../../../internal/testkit/boundary/e2e/boundary_test.go) | Production and E2E import guards live beside boundary tooling. The moved parents retain their names. Two narrow checks now scan every Go source file to enforce exact testkit tool owners and forbid integration-to-E2E-internal imports. |
| `internal/testfeedback` | [`internal/testkit/feedback`](../../../internal/testkit/feedback/summary.go) | Go test event, package, test, failure, skip, build-event, and completeness summaries; [`summary_test.go`](../../../internal/testkit/feedback/summary_test.go) remains package-local. |
| `internal/testprocess` | [`internal/testkit/process`](../../../internal/testkit/process/process.go) | Test subprocess lifecycle and descendant cleanup; Unix/Windows implementations and their package tests move together. The process implementation changes only package/path ownership. |
| `internal/steerline/testprotocol` | [`internal/testkit/protocol`](../../../internal/testkit/protocol) | Already delivered by #80. No duplicate migration or behavior change in #81. The exact #80 source and verification history remains in [#80 ownership](../three-line-08-app-host/ownership.md) and [#80 test mapping](../three-line-08-app-host/test-mapping.json). |
| `internal/steerlineintegration/**`, `internal/steerline/integration/**` | [`tests/integration`](../../../tests/integration) | Already delivered by #80. The current directory has 34 Go files; #81 does not remigrate them. See the #80 inventory and its preserved run history in the linked files above. |

The final package inventory is `internal/testkit/{boundary,boundary/e2e,feedback,process,protocol}` plus `tests/integration` and the registered `tests/e2e` packages. No `internal/coreboundary`, `internal/testboundary`, `internal/testfeedback`, `internal/testprocess`, or old `internal/steerline/testprotocol` source directory remains. All `TestMC*` coverage uses its actual #80 owner under `internal/execution` or `tests/integration`; the retired `internal/steerline/candidate_workers_test.go` path is not used.

## Consumers and selection

- `cmd/test-feedback` imports only `internal/testkit/feedback` and `internal/testkit/process` from testkit. Its quick rules now select `internal/testkit/boundary/e2e`; its execution-boundary owner selects `internal/testkit/boundary`; its cross-module representative group remains `tests/integration`.
- The boundary source guard rejects every non-test production import of testkit except the exact `cmd/test-feedback` feedback/process pair and the existing E2E helper packages' process imports. It does not exempt `cmd/*` or production packages generally. Test-only protocol fixture imports remain test-only.
- E2E's existing driver, controlled, native, testenv and sandbox setup helpers now import `internal/testkit/process`. Scenario packages continue to use the existing driver/evidence adapters to reach product entry points.
- [`tests/integration/core_decoupling_67_boundary_test.go`](../../../tests/integration/core_decoupling_67_boundary_test.go) now imports the checker from `internal/testkit/boundary`; `TestCD67CoreBoundary` and its assertion hierarchy remain in `tests/integration`.
- Full still obtains all packages from `go list ./...` and partitions only the execution business package. `TestFullPackageEnumerationIncludesTestkitOwners` protects that inventory for boundary, boundary/e2e, feedback, process and protocol.
- [`tests/e2e/suites.json`](../../../tests/e2e/suites.json) remains unchanged: the ten scenario IDs, package/test names, level, environment and live chain fields are preserved. Platform build tags remain on the Unix/Windows process implementations and Windows process test.

The complete moved parent/subtest and assertion map, actual selected names, and commit-bound run evidence are in [`test-mapping.json`](test-mapping.json). Current E2E boundary and environment guidance is in [`tests/e2e/README.md`](../../../tests/e2e/README.md); the current source index links back to this inventory in [`current-code-architecture.md`](../../architecture/current-code-architecture.md).
