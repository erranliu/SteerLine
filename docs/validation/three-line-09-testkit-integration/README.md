# Task 81 validation

Task 81 starts at `e3ef412fd9bd00aab2b8f06c62d97686f937b44d`, the merge commit of #80 / [PR225](https://github.com/erranliu/SteerLine/pull/225). It moves the remaining boundary, feedback and process support to `internal/testkit`, updates consumers and test selection, and keeps the already delivered #80 cross-module tests in `tests/integration`.

The source/test inventory is in [ownership.md](ownership.md); [test-mapping.json](test-mapping.json) records every moved parent/subtest and assertion owner, the fixed L1/Full/E2E paths, and commit-bound commands. The plan is [three-line-09-testkit-integration.plan.md](../../archive/spec/completed/three-line-09-testkit-integration.plan.md). The #80 report and its pass/fail history remain unchanged at [three-line-08-app-host](../three-line-08-app-host/README.md).

## Validation results

Implementation source commit: `576dd464ffc3586c5da8d4cbd72d9316b598df76`. Every passing summary records that SHA and a clean worktree apart from its own excluded output directory. Each runner used a fresh empty output directory and its own short Windows temp `STEERLINE_HOME`; environment values were restored after each command.

| Check | Result | Evidence |
| --- | --- | --- |
| Targeted runner, Full owner inventory, boundary/feedback/process | Passed: 5 packages, 108/108 tests, 0 fail/skip; 11.7s runner time | [summary](runner/commit-bound-02/summary.json), [Go JSON](runner/commit-bound-02/go-test.jsonl) |
| Fixed `quick-boundary` real app group | Passed: 1 package, 59/59 tests, 0 fail/skip; 692.1s runner time | [summary](integration/commit-bound-01/summary.json), [Go JSON](integration/commit-bound-01/go-test.jsonl) |
| E2E tag compile only | Passed: 11 E2E-tagged packages compiled; zero scenario bodies executed; 10 registry IDs unchanged | [build log](e2e-build/commit-bound-01/go-test.log), [metadata](e2e-build/commit-bound-01/run-metadata.json) |
| Fixed L1 | Passed: 12 groups, 27 packages, 459/459 tests, 0 fail/skip; 977.1s group execution | [runner summary](l1/commit-bound-01/summary.json), [group logs](l1/commit-bound-01/) |

The original fixed L1 selection is intact: 457 existing tests passed, plus the two new source-owner and integration/E2E-internal boundary assertions. In `quick-boundary`, `TestTM04ExecutionWiring` passed its original three subtests (`claim_transaction_failure_and_original_receipt`, `intent_binding_and_two_original_turns`, `original_request_ack_and_manual_recovery`); the app/host composition and default/explicit requirements claims also passed.

All five short task temp homes were removed after verification and `STEERLINE_HOME` was restored. The integration run's one `execution.json` was recorded by size and SHA256 before cleanup; duplicate invocation logs were hash-compared with their evidence copies before removal. See [cleanup-manifest.json](cleanup-manifest.json).

The first targeted attempt against `8865040b509952858c169a420a1251e875ed7ccc` failed because two relocated guard tests ascended one directory too far when finding the repository root, and the moved E2E guard retained an unused import/local. Its 100/103 passes, 3 failures, zero skips, build diagnostics and raw JSON are preserved at [runner/commit-bound-01](runner/commit-bound-01/). The root paths and unused locals were fixed in `576dd464…`; the fresh, non-overwriting run at `runner/commit-bound-02/` passed.

Full, L2, L3/live and native acceptance were not selected. E2E compilation is build evidence only. #38/#39 native reports and #80's earlier failed reports retain their original status and location; this task changes no historical validation result.
