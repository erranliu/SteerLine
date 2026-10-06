# TM10.1 Independent owner proofs

These proofs run at the #82 source commit, on Windows/amd64, without native opt-in or model calls. Existing product rules remain committed. Each mutation is supplied only through Go `-overlay`; the same actual owner test is then run with no overlay. Full before/after source and exact overlay patches are stored alongside each probe's runner output.

## Task selection and priority

- Production path: `task.Service.Ready` → `task/model.ReadyTasks` → `StableTaskOrder`.
- Owner test: `internal/task/model/taskgraph_test.go::TestStableTaskOrderUsesPriorityTimeAndNumericID`.
- Rule observed: priority, creation time and numeric ID determine order among currently unblocked tasks; task graph edges still constrain topological order.
- Overlay change: invert only the priority comparison. The test must report an order different from `[2, 10, 11, 9, 12]` and fail at the existing assertion.
- Full source before/after, patch, overlay manifest, source/tree fingerprints, test feedback and post-overlay unmodified run: [`probes/task`](probes/task/).

## Execution recovery and capacity

- Production paths: `Service.resumeWorkerFlow` uses `resumeRegisteredWorkerFlow` for active runs, aggregates per-run errors, and continues healthy siblings; `CommandService.StartNativeCandidate` checks current participation at each start.
- Owner tests: `internal/execution/candidate_resume_isolation_test.go::TestCandidateResumeErrorDoesNotStarveSibling` and `internal/execution/candidate_workers_test.go::TestMC9`.
- Preserved rules: a broken candidate retains its original diagnostic while a healthy sibling resumes; MC9 checks request replay, start-time participation limit, immediate release after exit and full history without a database slot constraint.
- The first overlay returned a retired-candidate resume error immediately, but the selected test parent did not exercise that branch; its real result was 2/2 passing. It is retained as a non-triggering diagnostic at [`execution-mutated-20261006-01`](independence/execution-mutated-20261006-01/) and is not counted as proof.
- Corrected overlay: return only the active `resumeRegisteredWorkerFlow` error before visiting later siblings. The real result was MC9 pass and sibling recovery fail at its healthy-sibling assertion. Full before/after source, patch, overlay manifest, source/tree fingerprints, test feedback and post-overlay unmodified owner run: [`probes/execution-active-branch`](probes/execution-active-branch/) and [active mutation output](independence/execution-active-branch-mutated-20261006-01/), [restored output](independence/execution-active-branch-restored-20261006-01/).

## Console navigation, input and display

- Production path: `console.Model` receives `product/api` snapshots and an injected client; decision key handling validates current project, service instance, run revision, decision version, identity and blocker before dispatch.
- Existing owner test: `internal/console/interaction_test.go::TestUX4RejectPreservesDraftAndBlockerStaysIndependent` uses the real product DTO fixture and console model. The app integration parent `tests/integration/candidate_console_test.go::TestCandidateConsoleWiring` exercises the actual app, client, SQLite, service and console construction path.
- Overlay change: remove only the environmental blocker submit barrier. UX4 must fail because the blocked answer is submitted rather than retained as a draft. The real app/client/SQLite fixture parent passed before the console overlay; mutation and restoration rerun the owner UX4 parent that consumes the product API fixture.
- Full source before/after, patch, overlay manifest, source/tree fingerprints and all test feedback: [`probes/console`](probes/console/), [baseline including real app wiring](independence/console-before-20261006-01/), [mutation](independence/console-mutated-20261006-01/), and [restored owner test](independence/console-restored-20261006-01/).

The fixed TM10.1 acceptance is unchanged. These are owner-specific rule probes, not a claim that a shared product semantic changed without contract coordination. No mutation is present in the final source tree.
