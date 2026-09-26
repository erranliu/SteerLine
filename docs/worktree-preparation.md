# Native task worktree preparation

SteerLine owns task claims, fixed workflow steps, execution identities and delivery in the center SQLite database. Codex and Orca perform only the assigned step in the prepared worktree. A project config must explicitly set `task_authority` to `native`; there is no Beads task source or whole-run handoff.

## State and worktree identity

Native state uses the repository's unique derived directory under `<STEERLINE_HOME>/state/<repoKey>`. A second `run_dir` for the same Git common-dir is rejected. The repository must not contain Beads state, and an existing database or runtime projection must match the native project and current schema before any service write.

For a claimed task, SQLite records the `WorkflowRun`, `Execution`, first `StepAttempt`, fixed workflow definition and worktree intent before preparation begins. The service prepares and verifies the workspace, then persists its path, branch, base and worker identity before launching the step. Preparation and execution share one slot. An operation with an unknown outcome retains its original identity and blocks new work until current-format reconciliation proves its result.

## Fixed step flow

The current `minimal-v1` sequence is `prepare`, `implement`, `verify`, optional configured `review`, `publish_pr`, then `waiting_pr_merge`. Each worker receives `native-task/v1` context for one task and one step, including project/task/contract identity, workflow/execution/attempt identity, repository/branch/base, prior step facts and this step's input. It does not receive task-source credentials or write task state.

Each step returns a structured result bound to its fixed identity. The center validates the result, repository state, prerequisite results and step-specific gates before recording success and creating the next attempt. The `publish_pr` result binds the PR URL and published head. A created PR does not complete the task: the center queries the exact PR and target branch, then atomically records delivery, closes the native Task and releases ready successors only after real merge evidence matches.

Local Codex and the supported Orca backend use this same task and step contract. Their process or terminal identities remain backend-specific and are reconciled against the persisted attempt. A result file or process exit alone cannot advance delivery.

## Failure and recovery

On restart, SteerLine reconciles saved Operations and step attempts against their exact task, execution, attempt and backend identities. It does not recreate a worktree or replay an action whose outcome is unknown. Missing, conflicting, unsupported or legacy task-source/result state is preserved and rejected before new side effects. No automatic cleanup removes worktrees, branches, PRs or external processes with uncertain identity.

The recovery panel is read-only until a supported action is explicitly submitted. Native failed-step retry reserves a new attempt through the center transaction. Unknown actions remain blockers until their original Operation can be reconciled; the retired one-shot `accept-result` path is not available. See [data contract](data-contract.md), [hosting](hosting.md), and the [execution architecture Spec](spec/execution-architecture.spec.md).
