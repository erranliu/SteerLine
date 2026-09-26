# Native task and execution state

SteerLine has one business task authority: the center SQLite database. It owns native Tasks, claims, dependencies, WorkflowRuns, Executions, StepAttempts, decisions, Operations and delivery. The runtime JSON is a rebuildable observation projection. PR platforms own merge facts; SteerLine records delivery only after it rechecks those facts.

## Independent state axes

| Axis | Fields | Authority | Meaning |
| --- | --- | --- | --- |
| Task | `task.status`, `claim` | Center SQLite | Whether the task is ready, claimed, closed or discarded |
| Execution | `execution.status`, current step and attempt | Center SQLite plus the bound backend identity | Whether a step is planned, running, succeeded, failed or unknown |
| Delivery | `workflow_run.phase`, `task_delivery` | Center SQLite after PR verification | Whether the workflow is active, waiting for a decision, waiting for merge, complete or blocked |
| Decision | `decision.status` and immutable answer | Center SQLite | Whether the answer is pending, saved, accepted by the backend, proven applied, failed or superseded |
| Dispatch | mode, pause, authority revision | Center SQLite | Whether new ready tasks may be claimed |

Process and terminal state are external observations. A process exit does not prove a step result; an accepted answer does not prove application; a created PR does not prove delivery. SteerLine keeps each fact separate and checks its fixed task/workflow/execution/attempt identity before advancing.

## Native workflow

New native projects start in manual, paused dispatch mode. A successful claim transaction creates the Task claim, WorkflowRun, Execution, prepare StepAttempt and operation checkpoint together. The worker receives a fixed `native-task/v1` context for one step. Each structured step result is validated against the saved contract, repository state, prerequisites and that step's gate before success and the next attempt are committed.

The current `minimal-v1` path prepares a workspace, implements, verifies required checks, optionally reviews, publishes a PR, and waits for merge. After a human merges, SteerLine re-queries the exact PR and branch. Only matching merge evidence lets one transaction record TaskDelivery, close the task and release ready successors. Duplicate delivery requests return the recorded delivery without repeating those effects.

Decision acceptance and application are separate. The answer is first-wins and immutable. A later step must return proof bound to the original decision, attempt, execution and answer digest before the decision becomes `applied`.

## Recovery and unsupported state

The runtime and database must match the repository, remote, branch, native authority and current schema. Old Beads state, retired whole-run result bindings, unsupported schema and unresolved external identities are preserved and rejected before task writes, dispatch, cleanup or worker launch. No migration, alternate authority, automatic worktree cleanup or blind process stop is attempted.

Current-format restart recovery uses saved Operations and attempts. An uncertain side effect remains attached to its original identity and reserves the slot until reconciliation supplies evidence. Failed native steps retry through a center transaction that reserves a new attempt; unknown work is not replayed. See [data contract](data-contract.md), [decision operations](decisions.md), and [worktree preparation](worktree-preparation.md).

## State location

`STEERLINE_HOME` defaults to `C:\SteerlineRuntime`. Native state is derived from the Git common-dir and stored under `state/<repoKey>`, shared across worktrees of that repository. The config must omit a custom `run_dir`; this prevents multiple local task authorities for the same repository. Runtime and service lifecycle details are in [hosting](hosting.md).
