# #82 Final consumer and deletion inventory

## Baseline

- Starting commit / `origin/main`: `9483afa13a9306e9852ef365c0746d1b5335c10f`, merge parents `e3ef412fd9bd00aab2b8f06c62d97686f937b44d` and `5aa714b74c66cadc9d5197413080c3a29d48ce1b`.
- The #81 predecessor is PR226, final head `5aa714b74c66cadc9d5197413080c3a29d48ce1b`, platform merge time `2026-10-06T08:31:38Z`; source and final testkit maps remain in [#81 ownership](../three-line-09-testkit-integration/ownership.md) and [#81 test mapping](../three-line-09-testkit-integration/test-mapping.json).
- This task consumes those completed moves. It does not recreate task, execution, product, console, storage, app, testkit or integration packages already moved by #73–#81.

## Current paths and consumers

| Candidate | Consumer finding | Decision |
| --- | --- | --- |
| `internal/steerline`, `internal/steerlineapp`, `internal/steerline/contracts`, `internal/steerlineintegration` | `go list ./...` has no package under those roots; current Go source has no imports of their former package paths. `rg --files internal` finds no source under those roots. The main executable imports `internal/app`, `internal/app/host`, `internal/execution`, `internal/product` and `internal/console`. | No alias or forwarding package remains to delete. Keep historical validation documents and recorded reports unchanged; old paths inside them identify the source tree that was tested at that time. |
| #81 testkit moves | `cmd/test-feedback` consumes `testkit/feedback` and `testkit/process`; E2E setup/driver/native helpers consume the process owner; boundary tests and protocol fixtures have their named consumers. The exact per-file and parent/subtest map is in #81's ledger. | Preserve all current consumers, assertions, OS variants and registered scenario IDs. |
| `internal/console/source.go` API type aliases | `ProjectRef`, `ProjectSnapshot` and `LogBatch` are used throughout current console production/test source as names for `product/api` values. They do not point at a retired package and console imports no implementation behind them. | Retain as used local API names; they are not compatibility shims. |
| `internal/storage/sqlite/values.go` aliases/constants | SQLite implementation and its tests use the current execution-model, product-api and storage-port values through these implementation-local names. | Retain current consumer-owned implementation declarations; no old package or duplicate implementation is exposed. |
| `internal/execution/adapters/orcaremote/types.go` `Message` alias | Current remote protocol request/response code uses the Orca message value. | Retain the existing protocol implementation and its tests; this is not authorization for first-phase remote product use. |
| `internal/app/config_fixture_test.go` aliases | Tests in package `app` use aliases to execution-owned profile/version declarations; no production source uses them. | Retain test-only fixture names; they do not add production API or behavior. |
| Historical fault-proxy source `docs/validation/pr-05-h15-orca-proxy-2026-09-28.go` | It is referenced by a preserved 2026-09-28 validation report and source manifests. It is not imported by product source, an installer, or current test adapter. `go list ./...` does enumerate the containing documentation package. | Preserve the exact historical source because the raw report identifies it as the proxy used for that experiment; do not treat it as a current helper or product dependency. |
| Native-test and console examples in `docs/testing.md` | Four native links and the console command named deleted `internal/steerline` paths. Their current tests live in `execution/adapters/orca`, `execution/adapters/orcaremote`, `app/host` and `internal/console`. | Update only the current test guide to current owners; leave historical validation/model/failure records unchanged. |

## Reproducible inventory checks

At source commit `9483afa13a9306e9852ef365c0746d1b5335c10f`, Windows `go list ./...` enumerated 42 packages, including the historical `docs/validation` source package. Filtering that list for `/internal/(steerline|steerlineapp|steerlineintegration)(/|$)` returned no rows. Searching Go imports in `cmd`, `internal` and `tests` for those module paths returned no rows. The resulting current package inventory is recorded in [`current-code-architecture.md`](../../architecture/current-code-architecture.md).

`type X = Y` declarations were inspected by owner and current use. The retained aliases above have concrete source consumers; none forwards from a retired package. The Windows/Linux and all-tag boundary checks stay in `TestCD67CoreBoundary` and the `internal/testkit/boundary` negative fixtures. No allowlist, no-op interface, second SQL writer, or compatibility path was added by #82.

The implementation diff deletes no Go behavior: at the #82 baseline, the former root packages and unused host/query/console forwarding packages were already absent. This inventory records the final verification and keeps the historical source artifacts that their reports explicitly cite.
