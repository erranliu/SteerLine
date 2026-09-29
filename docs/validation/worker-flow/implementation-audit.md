# Worker flow implementation audit

Date: 2026-09-29. Source revision: `2ce020b` (`origin/main`). This is a
pre-implementation audit, not WF1–WF12 acceptance evidence.

## Current production path

| Concern | Current authority and entry point | Required change |
| --- | --- | --- |
| Task claim | `ClaimNativeTask` in `task_execution.go` writes Task, WorkflowRun, Execution and a planned `prepare` StepAttempt in one transaction. | Keep the claim/CAS and DAG checks; stop creating a project-step chain. |
| Dispatch | `dispatchNative` in `native_service.go` prepares the workspace, then `startMinimalStep` / `resumeMinimalSteps` in `minimal_step.go` dispatch each step. | Start one logical worker conversation, retain its identity and workspace through turns. |
| Step result | `finishMinimalStep` validates a result envelope, workspace cleanliness and candidate revision, calls `AdvanceMinimalExecution`, then creates the next step attempt. | Treat turn completion, question and PR clues as observations; create one pending Decision when input is needed. |
| Feedback | `applyDecisions` in `decision.go` normally calls `resumeMinimalAfterDecision`, which creates another step attempt. The supervised path answers an in-flight Orca question but rejects option plus note. | Send the full answer to the same session with a durable message ID; record acceptance and turn start separately. |
| PR delivery | `pollNativePRs` only polls `waiting_pr_merge`; `validateNativeDeliveryGates` requires verify/review/publish results and a clean candidate SHA. | Verify a PR from session or user clues against repository, base and execution branch; complete only from provider merge evidence. |
| Recovery | `Service` v5 runtime recovery and supervised reattachment are keyed to the current StepAttempt and Dispatch. | Introduce a distinct v2 format and recover the original session/turn and in-flight message intent. Reject v5 before side effects. |
| Console | `console/model.go` still offers `retry_run`, `mark_stopped` and `resolve_command`; status projection includes blocked/unknown branches. | Expose one decision input and the five workflow phases, while retaining technical facts as diagnostics. |

## Platform interfaces observed

| Target | Installed interface evidence | Still to prove |
| --- | --- | --- |
| Windows local | `codex-cli 0.157.1`; [read-only probe](local-session-probe.json) records two `codex exec --json` turns with the same native session UUID and separate processes. A [host probe](windows-local-host-probe.json) found the current Job Object also contains a WMI-launched child. | An independently supervised launcher, three feedback rounds through SteerLine, durable send intent and exact-session recovery after service restart. |
| Orca local | `orca 1.4.209`; [isolated probe](orca-local-session-probe.json) records one terminal incarnation and native Codex session across the initial prompt and three accepted feedback turns, including a multiline option plus note. | SteerLine-owned send/observe/reconnect, host restart and full delivery. |
| Orca supervised remote | Existing `StartMinimalStep` makes a per-step Task/Dispatch and `settleSupervisedStepResources` releases it after a result. An [authenticated remote probe](orca-remote-session-probe.json) shows one Orca terminal incarnation and native Codex session across an initial prompt and three feedback turns, including option plus note. | SteerLine-owned durable remote send/observe/reconnect and full delivery; the old five-step report does not prove these. |

The local probe proves one same-session CLI continuation, not center delivery.
The Orca local and remote probes prove three same-session platform sends each,
not center delivery. None constitutes WF12 evidence. Do not switch production
configuration or rewrite the 308 project's
old 17/18/19 records on this basis.

## Integration overlap

At audit time, PR #115 (`codex/settled-run-reopen`) touches service recovery,
and PR #116 (`codex/durable-start-admission`) touches claim, dispatch, console
and service startup. Both are open. Integrate those changes serially before
claiming worker-flow validation on the final branch.
