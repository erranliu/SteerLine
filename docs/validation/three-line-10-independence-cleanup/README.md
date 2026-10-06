# #82 Independent development proof and final cleanup

Starting base is the true PR226/#81 merge `9483afa13a9306e9852ef365c0746d1b5335c10f` (parents `e3ef412fd9bd00aab2b8f06c62d97686f937b44d` and `5aa714b74c66cadc9d5197413080c3a29d48ce1b`; merged `2026-10-06T08:31:38Z`). The implementation branch is `codex/task-82-independence-cleanup`. It starts from current `origin/main`; #81 package and consumer moves are inherited, not replayed.

## Scope and owners

The current consumer/deletion audit is in [deletion-inventory.md](deletion-inventory.md). The final owner and test boundaries are in [ownership.md](ownership.md), and actual prior/new test parents, subtests and fixed TM10 mapping are in [test-mapping.json](test-mapping.json). Three real owner-rule probes and their mutation/restoration records are in [independence.md](independence.md).

The only user-visible documentation corrections are current package, build/install/launcher and test paths. No product behavior, persisted format, task/candidate rule, contract, runner registration, test skip, test interface, or native support claim is changed. The test-fixture correction scopes the in-process readiness read to the same existing `Project.Home` and provider-state path as spawned processes, then restores both environment values; it does not add a capability or scenario. Registered L3 assertions remain unchanged. Temporary successor-start probes were withdrawn after their single control receipt hit the existing operation-reconciliation barrier; both failures, source patches and receipts are preserved as diagnostics and are not counted as acceptance passes.

## Commit-bound verification

This section is completed from the exact source commit's `test-feedback` outputs. For every run, preserve the command, true process exit, source commit/tree, environment, executable/config, exact selection, parent/subtest counts and cleanup evidence. A probe mutation's expected failure is reported as a negative-rule result and never counted as a product pass.

| Scope | Source commit/tree | Actual selected parent/scenario | True exit and result | Evidence |
| --- | --- | --- | --- | --- |
| task owner baseline / overlay / restored | d87a714 / task owner | 1 pass / expected failure / 1 pass; 0 skip | [task probe](probes/task/) |
| execution owner baseline / overlay / restored | d87a714 / execution owner | 2 pass / expected 1 failure in corrected overlay / 2 pass; initial non-triggering overlay preserved; 0 skip | [execution probe](probes/execution/) |
| console/product fixture baseline / overlay / restored | d87a714 / console and integration owners | 3 pass / expected UX4 failure / 1 pass; 0 skip | [console probe](probes/console/) |
| L1 pre-final source | d87a714 / L1 | 12 groups; 459 pass, 0 fail/skip | [short-temp L1 run](l1-20261006-02/) |
| L1 final source | pending | pending | [final L1 run](l1-20261006-07/) |
| L3 controlled `delivery.basic` final source | pending | pending | [delivery run](delivery-20261006-06/) |
| L3 controlled `recovery.waiting-merge-restart` final source | pending | pending | [recovery run](recovery-20261006-02/) |
| Successor-start probe diagnostic | d5 / temporary test-source mutations | delivery and recovery receipts failed at original-operation reconciliation; no retry; not acceptance | [delivery diagnostic](delivery-20261006-05/), [recovery diagnostic](independence/recovery-successor-probe-20261006-01/) |

TM10.2 requires the original all-tag/negative boundary guards and L1; TM10.3 requires both registered controlled scenes with nonzero body run, exact request/identity evidence, unique delivery and complete cleanup. Controlled evidence does not certify #39 native execution. Full, L2, live and native opt-in are not selected by the fixed Plan.

## Final source and PR

| Field | Value |
| --- | --- |
| Source commit (implementation before runs) | pending |
| Source tree | pending |
| Final head | pending |
| Base | `9483afa13a9306e9852ef365c0746d1b5335c10f` at start; latest `origin/main` rechecked before PR |
| Implementation commits | pending |
| PR | pending |

Final source changes between the recorded source commit and tested candidate may contain only these validation artifacts; reports preserve actual failures and cleanup diagnostics. Remaining native limitations and any manual acceptance are stated explicitly. The execution session stops writing after PR publication and does not merge it.
