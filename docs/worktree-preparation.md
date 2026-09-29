# Native task worktree preparation

SteerLine owns task claims, execution identities and delivery in the center SQLite database. A project config must set `task_authority` to `native` and `workflow_protocol` to `worker-flow-v1`. The worker uses the prepared worktree and its bound session to implement the task and respond to feedback.

## State and worktree identity

Native state uses the repository's unique derived directory under `<STEERLINE_HOME>/state/<repoKey>`. A second `run_dir` for the same Git common-dir is rejected. The repository must not contain Beads state, and an existing database or runtime projection must match the native project and current schema before any service write.

For a claimed task, SQLite records the `WorkflowRun`, `Execution`, worker `StepAttempt` and worktree intent before preparation begins. The service prepares and verifies the workspace, then persists its path, branch, base and worker identity before launching the worker. Preparation and execution share one slot. An operation with an unknown outcome retains its original identity and blocks new work until current-format reconciliation proves its result.

## Worker flow

The current `worker-flow-v1` claim fixes project, task contract, workflow, execution, worker attempt, repository, branch and base identities. The same worker session can receive feedback and continue on the same task. It cannot write center task state.

The center associates a PR only after querying its repository, target branch and execution branch. A created PR does not complete the task: the center queries the exact PR, then atomically records delivery, closes the native Task and releases ready successors only after real merge evidence matches.

The active local worker's process identity is reconciled against its persisted attempt. Orca adapters and probes remain in the codebase, but new Orca claims are not enabled. A result file or process exit alone cannot advance delivery.

## Failure and recovery

On restart, SteerLine reconciles saved Operations and the worker attempt against their exact task, execution, attempt and backend identities. It does not recreate a worktree or replay an action whose outcome is unknown. Missing, conflicting or unsupported old state is preserved and rejected before new side effects. Cleanup requires exact resource identities and confirmed state.

The recovery panel reports original bindings and unresolved operations. Unknown actions remain blockers until their original Operation can be reconciled; old five-step retry and acceptance commands are unavailable. See [data contract](data-contract.md), [hosting](hosting.md), and the [worker-flow Spec](spec/worker-flow.spec.md).
