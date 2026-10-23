# Native local and Orca execution

This document describes the current native execution contract for local Codex and the supported Orca terminal backend. It is subordinate to the [execution architecture](../../../spec/execution-architecture.spec.md) and [native task constraints](../../../spec/native-task-constraints.md). There is one task authority and one fixed step protocol; backend identity describes where a step runs.

## Configuration

Every project must explicitly set `task_authority` to `native`. Native configuration requires the repository, remote, branch, PR provider, a supported `minimal-v1` workflow and its `project_steps`. It omits `run_dir`; SteerLine derives the repository's unique state directory. Unknown retired fields such as `bd`, `start_skill`, `finish_skill` and `completion` are rejected by config decoding.

`host` selects `local` or `orca`; `agent`, Codex model options and Orca CLI settings configure the selected executor. These values do not select a task source or change the task contract. The project steps define implement, verify, optional review and publish_pr. Verification checks are explicit and fixed in the workflow definition.

## Dispatch and step execution

The center transaction claims one ready native Task and creates its WorkflowRun, Execution, prepare StepAttempt, command receipt and Operation. The service records the worktree intent before creating the workspace. It verifies the resulting repository, branch and base, persists the worker identity, and only then starts the step.

Each worker receives `native-task/v1`: fixed project/task/contract, workflow/execution/attempt, repository/branch/base, prerequisite result references and one step's input. A worker cannot claim or close tasks, write the center database, change the workflow, or invoke Beads. The worker returns one structured step result. The center checks its identity, operation, repository state and step gate before recording the result and advancing.

Local Codex and Orca use the same center transitions and step result contract. Their process/terminal handles, output collection and stop capabilities stay backend-specific. A backend may be used only for the exact target/protocol/version combination supported by its capability evidence. Unsupported or uncertain actions retain the original binding and block replacement work.

## Delivery and decisions

`publish_pr` reports a PR URL and published head as step facts. The center verifies the saved step result, required verification/review gates and current PR identity. Human review and merge remain external. On a later poll, the center re-queries the precise repository, PR, target branch and merge commit; one SQLite transaction records TaskDelivery, closes the task and releases successors. A failed query, changed head or conflicting delivery leaves the task open and preserves its blocker.

Decision answers are stored once in the center. Answer acceptance is not application: the resumed native step must return evidence bound to the original decision, attempt, execution and answer digest. The service resumes the same workflow and execution with a new attempt after checking the saved workspace and waiting step; it does not create a replacement task or workflow.

## Restart and recovery

Restart reconciliation reads only current SQLite state and saved Operation/attempt identities. It does not recreate worktrees, replay uncertain worker starts, or import old result files. Native failed-step retry is a center transaction that reserves a new attempt; unverified in-flight actions remain blockers. The retired one-shot result acceptance and Beads claim/close paths are not part of this contract.

Old runtime or database versions, legacy task-source fields, retired whole-run result bindings, or unsupported external execution identities are rejected before side effects and retained for manual inspection. The service never treats a config change as permission to create a second authority.

## Acceptance evidence

`internal/steerline/native_tasks_test.go` covers local and Orca fixed-step flows, task claim/delivery and successor release, decision application, retry, and restart reconciliation. These isolated integration tests do not establish live Orca or Codex acceptance. Remaining live checks are tracked separately in the [roadmap](../../../Roadmap/drafts/roadmap.md) and the corresponding validation reports.
