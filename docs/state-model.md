# Native task and execution state

Current phase-1 execution and acceptance cover local Codex and local Orca only. Retained remote protocol/state definitions are future or historical material, not current remote support or a phase-1 gate. Git remote identity remains required for local PR delivery.

SteerLine has one business task authority: the center SQLite database. It owns native Tasks, claims, dependencies, WorkflowRuns, Executions, StepAttempts, decisions, Operations and delivery. The runtime JSON is a rebuildable observation projection. PR platforms own merge facts; SteerLine records delivery only after it rechecks those facts.

## Independent state axes

| Axis | Fields | Authority | Meaning |
| --- | --- | --- | --- |
| Task | `task.status`, `claim` | Center SQLite | Whether the task is ready, claimed, closed or discarded |
| Execution | `execution.status`, worker attempt and session | Center SQLite plus the bound backend identity | Whether the worker is planned, running, idle or uncertain |
| Delivery | `workflow_run.phase`, `task_delivery` | Center SQLite after PR verification | Whether the workflow is active, waiting for a decision, waiting for merge, complete or blocked |
| Decision | `decision.status` and immutable answer | Center SQLite | Whether the answer is pending, saved, accepted by the backend, proven applied, failed or superseded |
| Dispatch | mode, pause, authority revision | Center SQLite | Whether new ready tasks may be claimed |

Process and terminal state are external observations. A process exit does not prove a step result; an accepted answer does not prove application; a created PR does not prove delivery. SteerLine keeps each fact separate and checks its fixed task/workflow/execution/attempt identity before advancing.

## Native workflow

当前 `worker-flow-v1` 在领取时固定 WorkflowRun、Execution、worker Attempt 和工作区意图。服务在原会话中接收反馈、关联 PR；真实合并后，中心事务记录 TaskDelivery、关闭 Task 并释放后继。重复交付请求只回读已保存的交付。

以下 Orca Dispatch 细节是历史平台设计记录；当前能力报告仍未开放生产 claim。受监管 Orca Run 同时保存本地 WorkflowRun/Execution/Attempt 与外部 Run/Task/Dispatch、执行 host、worktree 和 runtime epoch。外部动作有独立 Operation 与平台 request ID，身份不确定时保留占用，不派替代 worker。

Orca `worker-start` 的 `input_accepted` 只使 Attempt 到 `dispatched`。同一 Dispatch 的 assistant transcript 或业务消息被服务持久化后才转 `running`；terminal ready、idle、输出沉默或存活事实不证明 turn 开始。`dispatched` 的固定绑定也可在中心重启后重接并继续观察。

保留远端协议资料（非一期流程）：受监管远端 Run 的 `execution_status` 在 `input_accepted` 后也保持 `pending`，直到同一 Attempt 的 turn 证据到达才记为 `running`。这条业务阶段与 Orca 报告的进程 `live` 分开；中心重启时，已有精确 Process/Dispatch/Operation 绑定的 `pending/dispatched` 仍只读重接原 worker。

New native projects start in manual, paused dispatch mode. A successful claim transaction creates the Task claim, WorkflowRun, Execution, worker StepAttempt and operation checkpoint together. The worker receives a fixed task context and continues in the same session through feedback and PR changes. After a human merges, SteerLine re-queries the exact PR and branch. Only matching merge evidence lets one transaction record TaskDelivery, close the task and release ready successors.

Decision acceptance and application are separate. The answer is first-wins and immutable. A later step must return proof bound to the original decision, attempt, execution and answer digest before the decision becomes `applied`.

## Recovery and unsupported state

The runtime and database must match the repository, remote, branch, native authority and current schema. Old Beads state, retired whole-run result bindings, unsupported schema and unresolved external identities are preserved and rejected before task writes, dispatch, cleanup or worker launch. No migration, alternate authority, automatic worktree cleanup or blind process stop is attempted.

Current-format restart recovery uses saved Operations and worker attempts. An uncertain side effect remains attached to its original identity and reserves the slot until reconciliation supplies evidence; unknown work is not replayed. See [data contract](data-contract.md), [decision operations](decisions.md), and [worktree preparation](worktree-preparation.md).

## State location

`STEERLINE_HOME` defaults to `C:\SteerlineRuntime`. Native state is derived from the Git common-dir and stored under `state/<repoKey>`, shared across worktrees of that repository. The config must omit a custom `run_dir`; this prevents multiple local task authorities for the same repository. Runtime and service lifecycle details are in [hosting](hosting.md).
