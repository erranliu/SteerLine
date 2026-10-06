# #82 Independent development proof and final cleanup

Starting base is the true PR226/#81 merge `9483afa13a9306e9852ef365c0746d1b5335c10f` (parents `e3ef412fd9bd00aab2b8f06c62d97686f937b44d` and `5aa714b74c66cadc9d5197413080c3a29d48ce1b`; merged `2026-10-06T08:31:38Z`). The implementation branch is `codex/task-82-independence-cleanup`. It starts from current `origin/main`; #81 package and consumer moves are inherited, not replayed.

## Scope and owners

The current consumer/deletion audit is in [deletion-inventory.md](deletion-inventory.md). The final owner and test boundaries are in [ownership.md](ownership.md), and actual prior/new test parents, subtests and fixed TM10 mapping are in [test-mapping.json](test-mapping.json). Three real owner-rule probes and their mutation/restoration records are in [independence.md](independence.md).

The only user-visible documentation corrections are current package, build/install/launcher and test paths. No product behavior, persisted format, task/candidate rule, contract, runner registration, test skip, test interface, or native support claim is changed. The test-fixture correction scopes the in-process readiness read to the same existing `Project.Home` and provider-state path as spawned processes, then restores both environment values; it does not add a capability or scenario. Registered L3 assertions remain unchanged. Temporary successor-start probes were withdrawn after their single control receipt hit the existing operation-reconciliation barrier; both failures, source patches and receipts are preserved as diagnostics and are not counted as acceptance passes.

## Commit-bound verification

This section is completed from the exact source commit's `test-feedback` outputs. For every run, preserve the command, true process exit, source commit/tree, environment, executable/config, exact selection, parent/subtest counts and cleanup evidence. A probe mutation's expected failure is reported as a negative-rule result and never counted as a product pass.

Each L3 report records the compiled `steerline.exe` and controlled-helper SHA-256 values. [`build-artifact-hashes.json`](build-artifact-hashes.json) indexes the generated Windows executables by run and size; the scoped `.gitignore` keeps those per-run build products local while the raw build logs and fingerprints remain in the report.

| Scope | Source commit/tree | Actual selected parent/scenario | Before / mutation / restored counts | True exit and result | Evidence |
| --- | --- | --- | --- | --- | --- |
| task owner probe | d87a714 / b30d95e7 | `TestStableTaskOrderUsesPriorityTimeAndNumericID` | 1/1 pass / 1/1 fail / 1/1 pass; 0 skip | 0 / 1 / 0; completed negative-rule proof; mutation is not a product pass | [task probe](probes/task/), [before](independence/task-before-20261006-01/), [mutation](independence/task-mutated-20261006-01/), [restored](independence/task-restored-20261006-01/) |
| execution owner probe | d87a714 / b30d95e7 | `TestMC9`, `TestCandidateResumeErrorDoesNotStarveSibling` | 2/2 pass / MC9 pass and sibling recovery fail / 2/2 pass; 0 skip | 0 / 1 / 0; active-branch negative-rule proof; initial retired-branch overlay was 2/2 pass, exit 0, non-triggering diagnostic | [active-branch probe](probes/execution-active-branch/), [before](independence/execution-before-20261006-01/), [mutation](independence/execution-active-branch-mutated-20261006-01/), [restored](independence/execution-active-branch-restored-20261006-01/), [initial diagnostic](probes/execution/) |
| console/product fixture probe | d87a714 / b30d95e7 | baseline: `TestUX4RejectPreservesDraftAndBlockerStaysIndependent` and `TestCandidateConsoleWiring` (plus child); mutation/restored: UX4 only | 3/3 pass / UX4 1/1 fail / UX4 1/1 pass; 0 skip | 0 / 1 / 0; completed negative-rule proof; real app wiring passed in baseline only | [console probe](probes/console/), [before](independence/console-before-20261006-01/), [mutation](independence/console-mutated-20261006-01/), [restored](independence/console-restored-20261006-01/) |
| L1 pre-final source | d87a714 / b30d95e7 | 12 fixed L1 groups | 459/459 pass, 0 fail/skip | true exit 0 | [short-temp L1 run](l1-20261006-02/) |
| L1 first final-source attempt | b098dfa / 36273dab | 12 fixed L1 groups | 456 terminated tests, 5 fail, 0 skip | true exit 1; `quick-boundary` incomplete with `$GIT_DIR too big` | [preserved run](l1-20261006-07/) |
| L1 exact-child path diagnostic | b098dfa / 36273dab | selected integration parents/children recorded in invocation | 5/5 pass, 0 fail/skip | true exit 0; shortest temp root | [targeted diagnostic](independence/l1-path-diagnostic-final-20261006-01/) |
| L1 final source | b098dfa / 36273dab | 12 fixed L1 groups | 459/459 pass, 0 fail/skip | true exit 0 | [final L1 run](l1-20261006-08/) |
| L3 controlled delivery final source | b098dfa / 36273dab | `delivery.basic` / `TestBasicDelivery` | 1/1 pass, 0 fail/skip | true exit 0; cleanup complete | [delivery run](delivery-20261006-06/) |
| L3 controlled recovery final source | b098dfa / 36273dab | `recovery.waiting-merge-restart` / `TestWaitingMergeRestart` | 1/1 pass, 0 fail/skip | true exit 0; restart and cleanup complete | [recovery run](recovery-20261006-02/) |
| Successor-start probe diagnostics | 8da65c7f / e749b329; temporary delivery test and recovery overlays | `TestBasicDelivery`, `TestWaitingMergeRestart` | each 1/1 fail, 0 skip | each true exit 1; original-operation reconciliation barrier; no retry; not acceptance | [delivery diagnostic](delivery-20261006-05/), [recovery diagnostic](independence/recovery-successor-probe-20261006-01/) |

TM10.2 requires the original all-tag/negative boundary guards and L1; TM10.3 requires both registered controlled scenes with nonzero body run, exact request/identity evidence, unique delivery and complete cleanup. Controlled evidence does not certify #39 native execution. Full, L2, live and native opt-in are not selected by the fixed Plan.

## Registered coverage and additional successor diagnostics

The unchanged registered `delivery.basic` body verifies A's unique Delivery and B's dependency-ready state with no WorkflowRun; it does not start B. The final [completed snapshot](delivery-20261006-06/scenarios/delivery.basic/delivery-completed.json) and [final snapshot](delivery-20261006-06/scenarios/delivery.basic/delivery-final.json) contain only A's workflow/execution. The registered recovery run preserves A's original request, identity and receipt across restart, then records its unique Delivery and unlocks B.

The additional failed probes record a separate, already committed successor-base fact:

| Diagnostic | B WorkflowRun | B workspace base = A unique Delivery merge | Original evidence |
| --- | --- | --- | --- |
| delivery-20261006-05 | `8c810bcab2843bc60416df2c01cdea97` | `cba45956637797dbc4c86a7342c5f4b4afd81940` | [final snapshot](delivery-20261006-05/scenarios/delivery.basic/delivery-final.json), [raw log](delivery-20261006-05/e2e-1.jsonl), [invocation](delivery-20261006-05/invocation.json) |
| recovery-successor-probe-20261006-01 | `bd327a782b5600ffc79584960f65d0bf` | `279543c5d09df1db61f8faa0145098d4ec9981f5` | [final snapshot](independence/recovery-successor-probe-20261006-01/scenarios/recovery.waiting-merge-restart/recovery-final.json), [raw log](independence/recovery-successor-probe-20261006-01/e2e-1.jsonl), [invocation](independence/recovery-successor-probe-20261006-01/invocation.json) |

In both snapshots B's claim has been committed, workspace preparation and new session preparation have completed, and its claim request, workspace/attempt and lifecycle target retain that same fixed base and predecessor merge. B's `start_worker_turn` is `outcome_unknown` after `Codex app-server connection ended during turn/start: EOF`. The unchanged [controlled helper](../../../tests/e2e/internal/controlled/helper.go) retains provider-global `Dispatches` when starting a new thread and rejects `turn/start` whenever `Dispatches != 0`: A already consumed its single business dispatch. The second business turn therefore cannot complete in this fixture. The production [operation barrier](../../../internal/execution/service_commit.go) keeps the original request and requires reconciliation before another action; the probes did not retry or relax it. Their overall results remain failed and do not count as acceptance passes. These committed fixed-base observations are separate from the registered scene coverage.

## Source-index provenance limit

The preserved historical [source-index-b64305be.json](source-index-b64305be.json) records `tests/e2e/internal/testenv/env.go` SHA-256 `eafb722dccb0dafc54c1def658379cc713ebf3e683690725bcf3214ea25ebe11`. Its byte source is unconfirmed: it does not match the b64305be Git object under either pure LF or pure CRLF normalization. The other 795 entries match; this single historical index entry is not a final-source certificate. No mixed-EOL origin is asserted and the original index/hash is retained.

The final [source-index.json](source-index.json) is bound to source commit `b098dfa89de7bbd2bc4b3745465659de7aa1f1f2`, tree `36273dab6ee1095c7e1fc0df0ee388488042ef22`; all 796 entries match that Git source under the recorded byte/EOL checks and the independently recomputed final index. Final L1 and both registered L3 invocations are bound to this source, not the unresolved historical entry.

## Final source and PR

| Field | Value |
| --- | --- |
| Source commit (implementation before runs) | `b098dfa89de7bbd2bc4b3745465659de7aa1f1f2` |
| Source tree | `36273dab6ee1095c7e1fc0df0ee388488042ef22` |
| Current PR candidate | See the attached PR head for the current documentation/evidence candidate; source certification remains explicitly bound to b098dfa and its recorded tree above. |
| Base | `9483afa13a9306e9852ef365c0746d1b5335c10f` at start; latest `origin/main` rechecked before PR |
| Implementation/evidence lineage | Full commits and roles are recorded below; c88044c1 is the preserved evidence publication commit, not a claim about the current PR head. |
| PR | See the attached pull request artifact. |

Recorded lineage from actual base to the original evidence publication:

| Commit | Role |
| --- | --- |
| `d87a714baeea5aa8a8d0173deb5a232cf0ba1d47` | Current architecture, ownership and test-path documentation; three owner probe source baseline. |
| `fbad8755deebf84ce1358dce9d4469eadc5996a8` | Initial project-home readiness fix and its diagnostic documentation. |
| `b64305be4ba29cffd1a947abb4c8eaf21f181b47` | Complete testenv readiness context fix: project home/provider-state environment binding and restoration; final surviving Go implementation. |
| `6dbd1ca6fc61e91b1506345a33d9700ebce2b13b` | Temporary delivery successor-base probe and diagnostic documentation. |
| `8da65c7fb81dddc2300350b202efdf9d47e9f03e` | Temporary single start-task successor probe; source for failed successor diagnostics. |
| `b098dfa89de7bbd2bc4b3745465659de7aa1f1f2` | Withdraw temporary registered delivery assertions, preserve fixed acceptance and diagnostics; final tested source owner. |
| `c88044c1365bdc8d71e50ddddf012a8bf1ff8afc` | Publish validation artifacts and recorded Plan results; no Go/source implementation change after b098dfa. |

Final source changes between the recorded source commit and tested candidate contain the recorded validation artifacts and Plan result notes, followed by documentation/probe metadata corrections; reports preserve actual failures and cleanup diagnostics. Remaining native limitations and any manual acceptance are stated explicitly. The execution session stops writing after PR publication and does not merge it.
